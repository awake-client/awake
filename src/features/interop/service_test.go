package interop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clementherve/awake/src/domain"
)

type fakeWriter struct {
	collections  []string
	requests     map[string]domain.Request
	environments map[string]Environment
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{requests: map[string]domain.Request{}, environments: map[string]Environment{}}
}

func (writer *fakeWriter) CreateEnvironment(_ string, scope string, fileName string, environment Environment) error {
	writer.environments[scope+"/environments/"+fileName] = environment
	return nil
}

func (writer *fakeWriter) CreateCollection(_ string, relativePath string, _ domain.Collection) error {
	writer.collections = append(writer.collections, relativePath)
	return nil
}

func (writer *fakeWriter) CreateRequest(_ string, relativePath string, request domain.Request) error {
	writer.requests[relativePath] = request
	return nil
}

type fakeSource struct {
	collections []CollectionSnapshot
}

func (source fakeSource) LoadCollections(_ string) ([]CollectionSnapshot, error) {
	return source.collections, nil
}

func TestImportWritesCollectionsAndRequests(t *testing.T) {
	document := []byte(`
openapi: 3.1.0
info:
  title: Platform API
paths:
  /users:
    get:
      summary: List users
  /users/{id}:
    delete:
      summary: Delete user
`)

	sourcePath := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(sourcePath, document, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	writer := newFakeWriter()
	service := NewService(writer, fakeSource{})

	result, err := service.Import(t.TempDir(), sourcePath, FormatOpenAPI)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if result.Collections != 1 || result.Requests != 2 {
		t.Errorf("unexpected result: %+v", result)
	}
	if len(writer.collections) != 1 || writer.collections[0] != "collections/platform-api" {
		t.Errorf("unexpected collections: %v", writer.collections)
	}
	if len(writer.requests) != 2 {
		t.Fatalf("unexpected requests: %v", writer.requests)
	}
	if _, found := writer.requests["collections/platform-api/list-users.yaml"]; !found {
		t.Errorf("expected list-users request, got %v", writer.requests)
	}
}

func TestExportOpenAPIWritesAFile(t *testing.T) {
	service := NewService(newFakeWriter(), fakeSource{
		collections: []CollectionSnapshot{{
			Name: "Platform",
			Requests: []RequestSnapshot{{
				RelativePath: "users/list.yaml",
				Request: domain.Request{
					Name:   "List users",
					Method: domain.MethodGet,
					URL:    "{{baseUrl}}/users",
				},
			}},
		}},
	})

	destination := filepath.Join(t.TempDir(), "export")
	result, err := service.Export(t.TempDir(), destination, FormatOpenAPI, ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	expectedPath := destination + ".yaml"
	if len(result.Paths) != 1 || result.Paths[0] != expectedPath {
		t.Fatalf("unexpected paths: %v", result.Paths)
	}

	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	collections, err := ReadOpenAPI(content)
	if err != nil {
		t.Fatalf("read exported openapi: %v", err)
	}
	if len(collections[0].Folders) != 1 || len(collections[0].Folders[0].Requests) != 1 {
		t.Errorf("unexpected exported tree: %+v", collections[0])
	}
}

func TestImportBrunoReadsEnvironmentsAsEnvironments(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"collection.bru":            "meta {\n  name: Registry\n}\n",
		"catalog/list.bru":          "meta {\n  name: List\n}\n\nget {\n  url: {{baseUrl}}/v2/_catalog\n}\n",
		"environments/registry.bru": "vars {\n  baseUrl: https://registry.example.com\n  username: clement\n  ~disabled: x\n  api-key: y\n}\nvars:secret [\n  password,\n  token\n]\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writer := newFakeWriter()
	result, err := NewService(writer, fakeSource{}).Import(t.TempDir(), root, FormatBruno)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if result.Requests != 1 || result.Environments != 1 {
		t.Errorf("unexpected result: %+v", result)
	}
	environment, found := writer.environments["collections/registry/environments/registry"]
	if !found {
		t.Fatalf("expected the registry environment, got %v", writer.environments)
	}
	if environment.Variables["baseUrl"] != "https://registry.example.com" || environment.Variables["username"] != "clement" {
		t.Errorf("unexpected variables: %v", environment.Variables)
	}
	if _, found := environment.Variables["disabled"]; found {
		t.Error("a disabled variable must be skipped")
	}
	if _, found := environment.Variables["api-key"]; found {
		t.Error("a variable with an invalid name must be skipped")
	}
	if strings.Join(environment.SecretVariables, ",") != "password,token" {
		t.Errorf("unexpected secrets: %v", environment.SecretVariables)
	}
	if len(result.Warnings) != 2 {
		t.Errorf("expected warnings for api-key and the secret values, got %v", result.Warnings)
	}
}

// Regression: a collection marked by bruno.json alone, picked through its
// parent directory, read its environments as requests.
func TestImportBrunoFindsCollectionsMarkedByTheirManifest(t *testing.T) {
	parent := t.TempDir()
	files := map[string]string{
		"Withings OTA/bruno.json":                  `{"version": "1", "name": "Withings OTA", "type": "collection", "ignore": ["node_modules", ".git"]}`,
		"Withings OTA/1 - Auth/login.bru":          "meta {\n  name: Login\n}\n\npost {\n  url: {{baseUrl}}/login\n}\n",
		"Withings OTA/environments/Production.bru": "vars {\n  baseUrl: https://example.com\n}\n",
		"Withings OTA/node_modules/dep/x.bru":      "not a request",
	}
	for name, content := range files {
		path := filepath.Join(parent, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writer := newFakeWriter()
	result, err := NewService(writer, fakeSource{}).Import(t.TempDir(), parent, FormatBruno)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Collections != 1 || result.Requests != 1 || result.Environments != 1 {
		t.Errorf("unexpected result: %+v", result)
	}
	if len(writer.collections) != 1 || writer.collections[0] != "collections/withings-ota" {
		t.Errorf("unexpected collections: %v", writer.collections)
	}
	if _, found := writer.environments["collections/withings-ota/environments/production"]; !found {
		t.Errorf("expected the production environment, got %v", writer.environments)
	}
}

func TestBrunoPathParametersRoundTrip(t *testing.T) {
	request, err := ReadBrunoFile([]byte("meta {\n  name: Publish\n}\n\npost {\n  url: {{base_url}}/api/articles/:articleId/publish\n}\n\nparams:path {\n  articleId: 42\n}\n\nbody:form-urlencoded {\n  title: test\n  ~draft: yes\n}\n"), "publish")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(request.PathParameters) != 1 || request.PathParameters[0] != (domain.PathParameter{Name: "articleId", Value: "42"}) {
		t.Errorf("unexpected path parameters: %+v", request.PathParameters)
	}
	if request.Body == nil || len(request.Body.Fields) != 2 || request.Body.Fields[1].Enabled {
		t.Errorf("unexpected form fields: %+v", request.Body)
	}

	written := requestFile(request)
	if !strings.Contains(written, "params:path {\n  articleId: 42\n}") || !strings.Contains(written, "~draft: yes") {
		t.Errorf("unexpected written request:\n%s", written)
	}
}

func TestImportBrunoSkipsRequestsItCannotRead(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"bruno.json":                "{\"name\": \"Game\"}",
		"Chat/BroadcastMessage.bru": "meta {\n  name: BroadcastMessage\n  type: grpc\n}\n\ngrpc {\n  url: {{grpcUrl}}\n  method: /chat/Broadcast\n}\n",
		"Player/Profile.bru":        "meta {\n  name: Profile\n}\n\nget {\n  url: {{baseUrl}}/profile\n}\n",
		"Player/Broken.bru":         "meta {\n  name: Broken\n}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := NewService(newFakeWriter(), fakeSource{}).Import(t.TempDir(), root, FormatBruno)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Requests != 1 {
		t.Errorf("expected the HTTP request only, got %+v", result)
	}
	warnings := strings.Join(result.Warnings, "\n")
	if !strings.Contains(warnings, "1 gRPC request skipped, not supported by Awake: Chat/BroadcastMessage.bru") ||
		!strings.Contains(warnings, "Player/Broken.bru skipped") {
		t.Errorf("unexpected warnings:\n%s", warnings)
	}
}

func exportTestSource() fakeSource {
	return fakeSource{collections: []CollectionSnapshot{
		{
			Name: "Platform",
			Requests: []RequestSnapshot{{
				RelativePath: "status.yaml",
				Request: domain.Request{
					Name:   "Status",
					Method: domain.MethodGet,
					URL:    "{{baseUrl}}/status",
				},
			}},
			Environments: []Environment{{
				Name:            "Production",
				Variables:       map[string]string{"baseUrl": "https://api.example.com"},
				SecretVariables: []string{"token"},
			}},
		},
		{
			Name: "Billing",
			Requests: []RequestSnapshot{{
				RelativePath: "invoices.yaml",
				Request: domain.Request{
					Name:   "List invoices",
					Method: domain.MethodGet,
					URL:    "{{baseUrl}}/invoices",
				},
			}},
		},
	}}
}

func TestExportOpenAPISplitWritesOneFilePerCollection(t *testing.T) {
	service := NewService(newFakeWriter(), exportTestSource())
	destination := filepath.Join(t.TempDir(), "export")

	result, err := service.Export(t.TempDir(), destination, FormatOpenAPI, ExportOptions{SplitCollections: true})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	expected := []string{
		filepath.Join(destination, "platform.yaml"),
		filepath.Join(destination, "billing.yaml"),
	}
	if strings.Join(result.Paths, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected paths: %v", result.Paths)
	}

	content, err := os.ReadFile(expected[0])
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	collections, err := ReadOpenAPI(content)
	if err != nil {
		t.Fatalf("read exported openapi: %v", err)
	}
	if len(collections) != 1 || collections[0].Name != "Platform" {
		t.Errorf("unexpected exported collection: %+v", collections)
	}
}

func TestExportPostmanWritesEnvironmentsNextToTheDocument(t *testing.T) {
	service := NewService(newFakeWriter(), exportTestSource())
	destination := filepath.Join(t.TempDir(), "awake.postman.json")

	result, err := service.Export(t.TempDir(), destination, FormatPostman, ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	environmentPath := filepath.Join(filepath.Dir(destination), "awake.postman.environments", "platform-production.postman_environment.json")
	if !containsPath(result.Paths, environmentPath) {
		t.Fatalf("expected %s in %v", environmentPath, result.Paths)
	}

	environment := readPostmanEnvironment(t, environmentPath)
	if environment.Name != "Production" {
		t.Errorf("unexpected environment name %q", environment.Name)
	}

	values := map[string]postmanEnvironmentValue{}
	for _, value := range environment.Values {
		values[value.Key] = value
	}
	if values["baseUrl"].Value != "https://api.example.com" || values["baseUrl"].Type != "default" {
		t.Errorf("unexpected baseUrl value: %+v", values["baseUrl"])
	}
	if values["token"].Type != "secret" || values["token"].Value != "" {
		t.Errorf("unexpected secret value: %+v", values["token"])
	}
}

func TestExportPostmanSplitWritesOneFilePerCollection(t *testing.T) {
	service := NewService(newFakeWriter(), exportTestSource())
	destination := filepath.Join(t.TempDir(), "export")

	result, err := service.Export(t.TempDir(), destination, FormatPostman, ExportOptions{SplitCollections: true})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	expected := []string{
		filepath.Join(destination, "platform.postman.json"),
		filepath.Join(destination, "billing.postman.json"),
		filepath.Join(destination, "environments", "platform-production.postman_environment.json"),
	}
	for _, path := range expected {
		if !containsPath(result.Paths, path) {
			t.Errorf("expected %s in %v", path, result.Paths)
		}
	}

	content, err := os.ReadFile(expected[0])
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	collections, err := ReadPostman(content)
	if err != nil {
		t.Fatalf("read exported postman: %v", err)
	}
	if len(collections) != 1 || collections[0].Name != "Platform" {
		t.Errorf("unexpected exported collection: %+v", collections)
	}
	if len(collections[0].Folders) != 0 || len(collections[0].Requests) != 1 {
		t.Errorf("expected requests at the root, got %+v", collections[0])
	}
}

type postmanEnvironmentFile struct {
	Name   string                    `json:"name"`
	Values []postmanEnvironmentValue `json:"values"`
}

type postmanEnvironmentValue struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

func readPostmanEnvironment(t *testing.T, path string) postmanEnvironmentFile {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read postman environment: %v", err)
	}

	var environment postmanEnvironmentFile
	if err := json.Unmarshal(content, &environment); err != nil {
		t.Fatalf("parse postman environment: %v", err)
	}
	return environment
}

func containsPath(paths []string, path string) bool {
	for _, candidate := range paths {
		if candidate == path {
			return true
		}
	}
	return false
}
