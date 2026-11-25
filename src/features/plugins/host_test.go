package plugins

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
)

const hookManifest = `version: 1
id: example
name: Example
pluginVersion: 1.0.0
main: dist/plugin.js
settings:
  - { key: greeting, type: string, label: Greeting, default: hello }
  - { key: token, type: secret, label: Token }
contributes:
  hooks: [beforeRequest, afterResponse]
  dynamicVariables: [{ name: $greeting }]
  authentication: [{ type: stamp, label: Stamp }]
  importFormats: [{ format: lines, label: Lines, extensions: [.txt] }]
  commands: [{ id: hello, title: Hello }]
`

const hookBundle = `awake.register({
	beforeRequest(context) {
		context.request.headers["X-Greeting"] = context.settings.greeting;
		context.request.headers["X-Saw-Secret"] = String("password" in context.variables);
		context.variables.fromPlugin = "yes";
	},
	afterResponse(context) { awake.log("status", context.response.status); },
	dynamicVariables: { $greeting: (context) => context.settings.greeting + " " + (context.settings.token || "nobody") },
	authentication: { stamp(context) { context.request.headers["X-Stamp"] = context.fields.who; } },
	importFormats: { lines: (context) => [{ name: context.fileName, requests: [] }] },
	commands: { hello() { awake.log("hello from the command"); } },
})`

func TestHostRunsOnlyApprovedPlugins(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.publishPluginWithBundle(firstCommit, "", hookManifest, hookBundle)
	fixture.workspace.plugins = []domain.PluginDeclaration{{ID: "example", Repository: testRepositoryURL, Commit: firstCommit}}
	if _, err := fixture.service.Download(testWorkspaceRoot, "example"); err != nil {
		t.Fatalf("download: %v", err)
	}

	request := testHookRequest()
	if logs := fixture.host.BeforeRequest(testWorkspaceRoot, &request, map[string]string{}, nil); len(logs) != 0 || len(request.Headers) != 0 {
		t.Fatalf("a plugin waiting for approval must not run, got headers %+v and logs %v", request.Headers, logs)
	}

	if _, err := fixture.service.Approve(testWorkspaceRoot, "example", firstCommit); err != nil {
		t.Fatalf("approve: %v", err)
	}
	variables := map[string]string{"password": "hunter2", "base": "https://example.com"}
	fixture.host.BeforeRequest(testWorkspaceRoot, &request, variables, []string{"password"})

	headers := headerValues(request)
	if headers["X-Greeting"] != "hello" {
		t.Errorf("expected the hook to run with the default setting, got %v", headers)
	}
	if headers["X-Saw-Secret"] != "false" {
		t.Errorf("a plugin without the secrets permission must not see secret variables, got %v", headers)
	}
	if variables["password"] != "hunter2" || variables["fromPlugin"] != "yes" {
		t.Errorf("hidden secrets must be kept and changes applied, got %v", variables)
	}

	logs := fixture.host.AfterResponse(testWorkspaceRoot, request, domain.ScriptResponse{StatusCode: 204}, variables, nil)
	if len(logs) != 1 || logs[0] != "[example] status 204" {
		t.Errorf("unexpected logs %v", logs)
	}
}

func TestHostGivesSecretVariablesToPluginsWithThePermission(t *testing.T) {
	fixture := newHostFixture(t)
	manifest := strings.Replace(hookManifest, "main: dist/plugin.js\n", "main: dist/plugin.js\npermissions: [secrets]\n", 1)
	fixture.installPlugin(manifest, hookBundle)

	request := testHookRequest()
	fixture.host.BeforeRequest(testWorkspaceRoot, &request, map[string]string{"password": "hunter2"}, []string{"password"})

	if headerValues(request)["X-Saw-Secret"] != "true" {
		t.Errorf("expected the plugin to see the secret variable, got %+v", request.Headers)
	}
}

func TestHostStopsRunningAPluginMovedToAnUnapprovedCommit(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.installPlugin(hookManifest, hookBundle)
	request := testHookRequest()
	fixture.host.BeforeRequest(testWorkspaceRoot, &request, map[string]string{}, nil)
	if len(request.Headers) == 0 {
		t.Fatal("expected the approved plugin to run")
	}

	fixture.publishPluginWithBundle(secondCommit, "", hookManifest, hookBundle)
	fixture.workspace.plugins[0].Commit = secondCommit
	if _, err := fixture.service.Download(testWorkspaceRoot, "example"); err != nil {
		t.Fatalf("download: %v", err)
	}

	request = testHookRequest()
	fixture.host.BeforeRequest(testWorkspaceRoot, &request, map[string]string{}, nil)
	if len(request.Headers) != 0 {
		t.Errorf("a pulled commit must not run before it is approved, got %+v", request.Headers)
	}
}

func TestHostComputesDynamicValuesWithSecretSettings(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.installPlugin(hookManifest, hookBundle)

	values, _, err := fixture.host.DynamicValues(testWorkspaceRoot, []string{"$greeting", "$unknown"})
	if err != nil {
		t.Fatalf("dynamic values: %v", err)
	}
	if values["$greeting"] != "hello nobody" || len(values) != 1 {
		t.Fatalf("unexpected values %v", values)
	}

	if err := fixture.service.SetSecretSetting(testWorkspaceRoot, "example", "token", "s3cret"); err != nil {
		t.Fatalf("set secret setting: %v", err)
	}
	if err := fixture.service.SetSecretSetting(testWorkspaceRoot, "example", "greeting", "hi"); !errors.Is(err, ErrInvalidSetting) {
		t.Errorf("expected a shared setting to be refused as a secret, got %v", err)
	}
	states, err := fixture.service.SecretSettings(testWorkspaceRoot, "example")
	if err != nil || len(states) != 1 || !states[0].IsSet {
		t.Fatalf("unexpected secret states %+v: %v", states, err)
	}
	values, _, err = fixture.host.DynamicValues(testWorkspaceRoot, []string{"$greeting"})
	if err != nil || values["$greeting"] != "hello s3cret" {
		t.Fatalf("expected the plugin to read its secret setting, got %v: %v", values, err)
	}
	if strings.Contains(strings.Join(fixture.workspaceFile(), "\n"), "s3cret") {
		t.Error("a secret setting must never reach awake.yaml")
	}

	if err := fixture.service.SetSecretSetting(testWorkspaceRoot, "example", "token", ""); err != nil {
		t.Fatalf("clear secret setting: %v", err)
	}
	if _, found := fixture.keychain.values[keychainServicePrefix+testWorkspaceID+"|example/token"]; found {
		t.Error("an empty value must remove the secret")
	}
}

func TestHostAuthenticatesWithAnEnabledPluginOnly(t *testing.T) {
	fixture := newHostFixture(t)
	httpRequest, err := http.NewRequest(http.MethodGet, "https://api.example.com/items", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.host.Authenticate(testWorkspaceRoot, "example", "stamp", httpRequest, nil); !errors.Is(err, ErrPluginNotEnabled) {
		t.Fatalf("expected ErrPluginNotEnabled, got %v", err)
	}

	fixture.installPlugin(hookManifest, hookBundle)
	if _, err := fixture.host.Authenticate(testWorkspaceRoot, "example", "stamp", httpRequest, map[string]string{"who": "ada"}); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if httpRequest.Header.Get("X-Stamp") != "ada" {
		t.Errorf("expected the stamp header, got %v", httpRequest.Header)
	}
	if _, err := fixture.host.Authenticate(testWorkspaceRoot, "example", "other", httpRequest, nil); !errors.Is(err, ErrUnknownContribution) {
		t.Errorf("expected ErrUnknownContribution, got %v", err)
	}
}

func TestHostListsFormatsAndRunsCommands(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.installPlugin(hookManifest, hookBundle)

	imports, exports := fixture.host.Formats(testWorkspaceRoot)
	if len(imports) != 1 || imports[0].Format != "plugin:example/lines" || imports[0].Extensions[0] != ".txt" || len(exports) != 0 {
		t.Fatalf("unexpected formats %+v %+v", imports, exports)
	}
	collections, err := fixture.host.Import(testWorkspaceRoot, "plugin:example/lines", "api.txt", []byte("https://a.example"))
	if err != nil || !strings.Contains(string(collections), `"name":"api.txt"`) {
		t.Fatalf("unexpected import %s: %v", collections, err)
	}

	if _, err := fixture.host.Import(testWorkspaceRoot, "lines", "api.txt", nil); !errors.Is(err, ErrUnknownContribution) {
		t.Errorf("expected an unqualified format to be refused, got %v", err)
	}
	if _, err := fixture.host.Export(testWorkspaceRoot, "plugin:example/lines", []byte("[]")); !errors.Is(err, ErrUnknownContribution) {
		t.Errorf("expected an import-only format to refuse exports, got %v", err)
	}

	logs, err := fixture.service.RunCommand(testWorkspaceRoot, "example", "hello")
	if err != nil || len(logs) != 1 || logs[0] != "hello from the command" {
		t.Fatalf("unexpected command result %v: %v", logs, err)
	}
	variables, err := fixture.service.DynamicVariables(testWorkspaceRoot)
	if err != nil || len(variables) != 1 || variables[0].Name != "$greeting" {
		t.Errorf("unexpected dynamic variables %+v: %v", variables, err)
	}
}

func TestServiceReportsAnApprovedPluginThatFailsToLoad(t *testing.T) {
	fixture := newHostFixture(t)
	fixture.installPlugin(hookManifest, `throw new Error("bundle exploded")`)

	views, err := fixture.service.ListPlugins(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if views[0].State != PluginFailed || !strings.Contains(views[0].Problem, "bundle exploded") {
		t.Errorf("expected a failed plugin with its problem, got %+v", views[0])
	}
	request := testHookRequest()
	if logs := fixture.host.BeforeRequest(testWorkspaceRoot, &request, map[string]string{}, nil); len(logs) != 0 {
		t.Errorf("a plugin that failed to load must not run, got %v", logs)
	}
}

type hostFixture struct {
	*serviceFixture
	host     *Host
	keychain *fakeKeychain
}

func newHostFixture(t *testing.T) *hostFixture {
	t.Helper()

	fixture := newServiceFixture(t)
	keychain := &fakeKeychain{values: map[string]string{}}
	host := NewHost(fixture.service.cache, fixture.trust, fixture.workspace, keychain, testAwakeVersion, http.DefaultClient)
	ConnectHost(fixture.service, host)
	return &hostFixture{serviceFixture: fixture, host: host, keychain: keychain}
}

// installPlugin publishes and installs a plugin from its repository, which
// approves it.
func (fixture *hostFixture) installPlugin(manifest string, bundle string) {
	fixture.t.Helper()
	fixture.publishPluginWithBundle(firstCommit, "", manifest, bundle)
	if _, err := fixture.service.Install(testWorkspaceRoot, PluginSource{Repository: testRepositoryURL}, firstCommit); err != nil {
		fixture.t.Fatalf("install: %v", err)
	}
}

// workspaceFile returns what the fake awake.yaml holds, flattened to text.
func (fixture *hostFixture) workspaceFile() []string {
	lines := []string{}
	for _, declaration := range fixture.workspace.plugins {
		for key, value := range declaration.Settings {
			lines = append(lines, key+"="+value)
		}
	}
	return lines
}

func testHookRequest() domain.Request {
	return domain.Request{Method: domain.MethodGet, URL: "https://example.com/items", Body: &domain.Body{Type: domain.BodyNone}}
}

func headerValues(request domain.Request) map[string]string {
	values := map[string]string{}
	for _, header := range request.Headers {
		values[header.Name] = header.Value
	}
	return values
}

type fakeKeychain struct {
	values map[string]string
}

func (keychain *fakeKeychain) Get(service string, account string) (string, bool, error) {
	value, found := keychain.values[service+"|"+account]
	return value, found, nil
}

func (keychain *fakeKeychain) Set(service string, account string, value string) error {
	keychain.values[service+"|"+account] = value
	return nil
}

func (keychain *fakeKeychain) Delete(service string, account string) error {
	delete(keychain.values, service+"|"+account)
	return nil
}
