package plugins

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
)

// exampleDirectory holds the example plugin of the documentation. These tests
// keep it working as the plugin API evolves.
var exampleDirectory = filepath.Join("..", "..", "..", "examples", "plugins", "hmac-signer")

func TestExamplePluginContributesEverythingItDeclares(t *testing.T) {
	runtime := loadExamplePlugin(t, map[string]string{"secret": "s3cret", "header": "Authorization", "algorithmLabel": "HMAC-SHA256"})

	httpRequest, err := http.NewRequest(http.MethodPost, "https://api.example.com/items?page=2", strings.NewReader(`{"name":"ada"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Authenticate("hmac", httpRequest, map[string]string{"keyId": "team"}); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	bodyDigest := sha256.Sum256([]byte(`{"name":"ada"}`))
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte("POST\nhttps://api.example.com/items?page=2\n" + hex.EncodeToString(bodyDigest[:])))
	expected := "HMAC-SHA256 keyId=team,signature=" + hex.EncodeToString(mac.Sum(nil))
	if authorization := httpRequest.Header.Get("Authorization"); authorization != expected {
		t.Errorf("unexpected signature\n got %s\nwant %s", authorization, expected)
	}

	requestID, _, err := runtime.DynamicVariable("$requestId")
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(requestID) {
		t.Errorf("unexpected request id %q: %v", requestID, err)
	}
	isoDate, _, err := runtime.DynamicVariable("$isoDate")
	if err != nil || !strings.HasSuffix(isoDate, "Z") {
		t.Errorf("unexpected date %q: %v", isoDate, err)
	}

	request := domain.Request{Method: domain.MethodGet, URL: "https://example.com", Body: &domain.Body{Type: domain.BodyNone}}
	if _, err := runtime.BeforeRequest(&request, map[string]string{}); err != nil || len(request.Headers) != 1 || request.Headers[0].Name != "X-Request-Id" {
		t.Errorf("expected the hook to add X-Request-Id, got %+v: %v", request.Headers, err)
	}

	log, err := runtime.RunCommand("show-settings")
	if err != nil || len(log.Lines) != 3 || log.Lines[2] != "secret: set on this computer" {
		t.Errorf("unexpected command output %v: %v", log.Lines, err)
	}
}

func TestExamplePluginRefusesToSignWithoutItsSecret(t *testing.T) {
	runtime := loadExamplePlugin(t, map[string]string{"header": "Authorization", "algorithmLabel": "HMAC-SHA256"})
	httpRequest, err := http.NewRequest(http.MethodGet, "https://api.example.com/items", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.Authenticate("hmac", httpRequest, map[string]string{"keyId": "team"})

	if err == nil || !strings.Contains(err.Error(), "set the signing secret") {
		t.Fatalf("expected a clear error without the secret, got %v", err)
	}
}

func TestExamplePluginImportsAndExportsURLLists(t *testing.T) {
	runtime := loadExamplePlugin(t, nil)

	imported, _, err := runtime.Import("urls", "api.txt", "# comment\nhttps://a.example/users\nPOST https://a.example/users\n\n")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	var collections []struct {
		Name     string `json:"name"`
		Requests []struct {
			Request struct {
				Method string `json:"method"`
				URL    string `json:"url"`
			} `json:"request"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(imported, &collections); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if collections[0].Name != "api" || len(collections[0].Requests) != 2 || collections[0].Requests[1].Request.Method != "POST" {
		t.Errorf("unexpected collections %s", imported)
	}

	exported, _, err := runtime.Export("urls", []byte(`[{"name":"API","requests":[{"name":"List","request":{"method":"GET","url":"https://a.example/users"}}],"folders":[{"name":"Admin","requests":[{"name":"Stats","request":{"method":"GET","url":"https://a.example/stats"}}]}]}]`))
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if exported != "# API\nGET https://a.example/users\nGET https://a.example/stats\n" {
		t.Errorf("unexpected export %q", exported)
	}
}

func TestExamplePluginViewsExistAndAnswer(t *testing.T) {
	manifest, err := ReadManifest(exampleDirectory)
	if err != nil {
		t.Fatalf("read the manifest: %v", err)
	}
	views := []string{manifest.Contributes.ResponseViewers[0].View, manifest.Contributes.RequestTabs[0].View}
	for _, view := range views {
		content, err := ReadFile(exampleDirectory, view)
		if err != nil || !strings.Contains(string(content), `postMessage({ type: "awake:ready" }, "*")`) {
			t.Errorf("expected %s to announce itself to Awake: %v", view, err)
		}
	}

	runtime := loadExamplePlugin(t, map[string]string{"header": "X-Signature"})
	result, _, err := runtime.InvokeView("signature", "canonical", `{"request":{"Method":"post","URL":"https://a.example","Body":{"Content":""}}}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	emptyDigest := sha256.Sum256(nil)
	expected := `{"header":"X-Signature","text":"POST\nhttps://a.example\n` + hex.EncodeToString(emptyDigest[:]) + `"}`
	if result != strings.ReplaceAll(expected, "\n", `\n`) {
		t.Errorf("unexpected canonical text %s", result)
	}
}

func TestExampleMarketplaceIsValid(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "marketplace", MarketplaceFileName))
	if err != nil {
		t.Fatal(err)
	}
	marketplace, err := ParseMarketplace(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, found := marketplace.Find("hmac-signer"); !found {
		t.Error("expected the example marketplace to list the example plugin")
	}
}

func loadExamplePlugin(t *testing.T, settings map[string]string) *Runtime {
	t.Helper()

	manifest, err := ReadManifest(exampleDirectory)
	if err != nil {
		t.Fatalf("read the manifest: %v", err)
	}
	if !manifest.SupportsAwake("0.11.0") || manifest.SupportsAwake("0.10.1") {
		t.Errorf("expected the example to need Awake 0.11.0")
	}
	bundle, err := ReadFile(exampleDirectory, manifest.Main)
	if err != nil {
		t.Fatalf("read the bundle: %v", err)
	}
	runtime, err := LoadRuntime(manifest, bundle, RuntimeOptions{Settings: settings})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return runtime
}
