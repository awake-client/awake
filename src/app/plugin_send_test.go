package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/collections"
	"github.com/awake-client/awake/src/features/git"
	"github.com/awake-client/awake/src/features/httpclient"
	"github.com/awake-client/awake/src/features/plugins"
	"github.com/awake-client/awake/src/features/secrets"
	"github.com/awake-client/awake/src/features/workspace"
)

const signerManifest = `version: 1
id: signer
name: Signer
pluginVersion: 1.0.0
main: dist/plugin.js
settings:
  - { key: secret, type: secret, label: Signing secret }
contributes:
  hooks: [beforeRequest]
  dynamicVariables: [{ name: $requestId }]
  authentication: [{ type: hmac, label: HMAC, fields: [{ key: keyId, type: string, label: Key id }] }]
`

const signerBundle = `awake.register({
	beforeRequest(context) { context.request.headers["X-Plugin"] = "signer"; },
	dynamicVariables: { $requestId: () => "request-42" },
	authentication: {
		hmac(context) {
			const signature = awake.crypto.hmacSha256(context.settings.secret, context.request.method + " " + context.request.url);
			context.request.headers["Authorization"] = "HMAC " + context.fields.keyId + ":" + signature;
		},
	},
})`

// TestPluginsSignARequestSentFromTheWorkspace installs a plugin from a git
// repository and sends a request of the workspace through the HTTP client,
// as the application wires them.
func TestPluginsSignARequestSentFromTheWorkspace(t *testing.T) {
	isolateGit(t)
	var received *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received = request
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	pluginRepository := filepath.Join(t.TempDir(), "signer")
	commitFiles(t, pluginRepository, map[string]string{
		plugins.ManifestFileName: signerManifest,
		"dist/plugin.js":         signerBundle,
	}, "v1.0.0")

	workspaceRoot := t.TempDir()
	if _, err := workspace.Create(workspaceRoot, workspace.CreateOptions{Name: "Platform API", Mode: workspace.ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	repository := collections.NewRepository(workspaceRoot)
	if err := repository.CreateCollection("collections/api", domain.Collection{Name: "API"}); err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if err := repository.CreateRequest("collections/api/items.yaml", domain.Request{
		Name:   "Items",
		Method: domain.MethodGet,
		URL:    server.URL + "/items/{{$requestId}}",
		Authentication: &domain.Authentication{
			Type:   domain.PluginAuthenticationType("signer", "hmac"),
			Fields: map[string]string{"keyId": "team"},
		},
		Body: &domain.Body{Type: domain.BodyNone},
	}); err != nil {
		t.Fatalf("create request: %v", err)
	}

	keychain := &memoryKeychain{values: map[string]string{}}
	cache := plugins.NewCache(t.TempDir(), git.BareRepositories{})
	trust := plugins.NewTrustStore(filepath.Join(t.TempDir(), "trust.yaml"))
	host := plugins.NewHost(cache, trust, pluginWorkspace{}, keychain, "dev", http.DefaultClient)
	pluginService := plugins.NewService(cache, trust, pluginWorkspace{}, "dev")
	plugins.ConnectHost(pluginService, host)

	source := plugins.PluginSource{Repository: "file://" + filepath.ToSlash(pluginRepository), Ref: "v1.0.0"}
	preview, err := pluginService.Preview(workspaceRoot, source)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := pluginService.Install(workspaceRoot, source, preview.Commit); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := pluginService.SetSecretSetting(workspaceRoot, "signer", "secret", "shh"); err != nil {
		t.Fatalf("set the secret setting: %v", err)
	}

	client, err := httpclient.NewService(requestSource{secrets: secrets.NewRepository(keychain)}, nil)
	if err != nil {
		t.Fatalf("create the HTTP client: %v", err)
	}
	httpclient.SetPluginHost(client, host)

	response, err := client.Send(workspaceRoot, "collections/api/items.yaml", "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	defer client.Close(response.ID)

	if received.URL.Path != "/items/request-42" || received.Header.Get("X-Plugin") != "signer" {
		t.Errorf("expected the dynamic variable and the hook, got %s %v", received.URL, received.Header)
	}
	if authorization := received.Header.Get("Authorization"); !strings.HasPrefix(authorization, "HMAC team:") || len(authorization) != len("HMAC team:")+64 {
		t.Errorf("expected an HMAC signature, got %q", authorization)
	}
}

// memoryKeychain keeps secrets in memory, standing in for the operating
// system keychain.
type memoryKeychain struct {
	values map[string]string
}

func (keychain *memoryKeychain) Get(service string, account string) (string, bool, error) {
	value, found := keychain.values[service+"|"+account]
	return value, found, nil
}

func (keychain *memoryKeychain) Set(service string, account string, value string) error {
	keychain.values[service+"|"+account] = value
	return nil
}

func (keychain *memoryKeychain) Delete(service string, account string) error {
	delete(keychain.values, service+"|"+account)
	return nil
}
