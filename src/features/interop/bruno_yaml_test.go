package interop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clementherve/awake/src/domain"
)

func TestReadBrunoYAMLDirectory(t *testing.T) {
	root := t.TempDir()

	writeBrunoYAMLTestFile(t, root, "opencollection.yml", `
opencollection: 1.0.0

info:
  name: carbure
request:
  auth:
    type: bearer
    token: "{{token}}"
  variables:
    - name: baseUrl
      value: http://localhost:8080
docs:
  content: Carbure API requests.
  type: text/markdown
bundled: false
`)

	writeBrunoYAMLTestFile(t, root, "Get stations.yml", `
info:
  name: Get stations
  type: http
  seq: 2

http:
  method: GET
  url: "{{baseUrl}}/carbure/api/stations"
  params:
    - name: page
      value: "1"
      type: query
    - name: draft
      value: "true"
      type: query
      disabled: true
    - name: id
      value: "129202"
      type: path
  headers:
    - name: Accept
      value: application/json
    - name: X-Legacy
      value: "1"
      disabled: true
  auth: inherit
`)

	writeBrunoYAMLTestFile(t, root, "Search stations.yml", `
info:
  name: Search stations
  type: http
  seq: 1

http:
  method: POST
  url: "{{baseUrl}}/carbure/api/stations/filter"
  body:
    type: json
    data: |-
      {
        "city": "MIONS"
      }
  auth: inherit
`)

	writeBrunoYAMLTestFile(t, root, "mcp/folder.yml", `
info:
  name: MCP
  type: folder
  seq: 3

request:
  auth: inherit

docs:
  content: Model Context Protocol requests.
  type: text/markdown
`)

	writeBrunoYAMLTestFile(t, root, "mcp/initialize.yml", `
info:
  name: initialize
  type: http
  seq: 1

http:
  method: POST
  url: "{{baseUrl}}/carbure/mcp"
  headers:
    - name: Accept
      value: application/json, text/event-stream
  body:
    type: json
    data: |-
      {
        "jsonrpc": "2.0",
        "id": 1
      }
  auth: inherit
`)

	// Environments are not requests and must be ignored.
	writeBrunoYAMLTestFile(t, root, "environments/local.yml", `
name: local
variables:
  - name: token
    value: secret
`)

	// Dot directories and dependencies are not part of the collection.
	writeBrunoYAMLTestFile(t, root, ".github/workflows/ci.yml", `
name: CI
on: push
`)
	writeBrunoYAMLTestFile(t, root, "node_modules/package/request.yml", `
info:
  name: dependency
  type: http
`)

	collections, err := ReadBrunoDirectory(root)
	if err != nil {
		t.Fatalf("read bruno yaml directory: %v", err)
	}
	if len(collections) != 1 {
		t.Fatalf("expected one collection, got %d", len(collections))
	}

	collection := collections[0]
	if collection.Name != "carbure" {
		t.Errorf("unexpected collection name %q", collection.Name)
	}
	if collection.Documentation != "Carbure API requests." {
		t.Errorf("unexpected documentation %q", collection.Documentation)
	}
	if collection.Variables["baseUrl"] != "http://localhost:8080" {
		t.Errorf("unexpected variables: %+v", collection.Variables)
	}
	if collection.Authentication == nil ||
		collection.Authentication.Type != domain.AuthenticationBearer ||
		collection.Authentication.Token != "{{token}}" {
		t.Errorf("unexpected authentication: %+v", collection.Authentication)
	}

	if len(collection.Requests) != 2 {
		t.Fatalf("expected two root requests, got %d: %+v", len(collection.Requests), collection.Requests)
	}
	// Requests are ordered by info.seq.
	search := collection.Requests[0]
	if search.Name != "Search stations" {
		t.Fatalf("unexpected first request %q", search.Name)
	}
	if search.Request.Body == nil || search.Request.Body.Type != domain.BodyJSON ||
		!strings.Contains(search.Request.Body.Content, `"city": "MIONS"`) {
		t.Errorf("unexpected search body: %+v", search.Request.Body)
	}

	stations := collection.Requests[1]
	if stations.Request.Method != domain.MethodGet {
		t.Errorf("unexpected method %q", stations.Request.Method)
	}
	if stations.Request.URL != "{{baseUrl}}/carbure/api/stations" {
		t.Errorf("unexpected URL %q", stations.Request.URL)
	}
	if len(stations.Request.Parameters) != 2 {
		t.Fatalf("expected two query parameters, got %+v", stations.Request.Parameters)
	}
	if stations.Request.Parameters[0].Name != "page" || !stations.Request.Parameters[0].Enabled {
		t.Errorf("unexpected first parameter: %+v", stations.Request.Parameters[0])
	}
	if stations.Request.Parameters[1].Name != "draft" || stations.Request.Parameters[1].Enabled {
		t.Errorf("unexpected disabled parameter: %+v", stations.Request.Parameters[1])
	}
	if len(stations.Request.Headers) != 2 ||
		!stations.Request.Headers[0].Enabled ||
		stations.Request.Headers[1].Enabled {
		t.Errorf("unexpected headers: %+v", stations.Request.Headers)
	}
	if stations.Request.Authentication == nil ||
		stations.Request.Authentication.Type != domain.AuthenticationInherit {
		t.Errorf("unexpected authentication: %+v", stations.Request.Authentication)
	}

	if len(collection.Folders) != 1 {
		t.Fatalf("expected one folder, got %+v", collection.Folders)
	}
	folder := collection.Folders[0]
	if folder.Name != "MCP" {
		t.Errorf("unexpected folder name %q", folder.Name)
	}
	if folder.Documentation != "Model Context Protocol requests." {
		t.Errorf("unexpected folder documentation %q", folder.Documentation)
	}
	if len(folder.Requests) != 1 {
		t.Fatalf("expected one folder request, got %+v", folder.Requests)
	}
	if folder.Requests[0].Request.Method != domain.MethodPost {
		t.Errorf("unexpected folder request: %+v", folder.Requests[0].Request)
	}
}

func TestReadBrunoDirectoryFindsNestedYAMLCollections(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"first", "second"} {
		writeBrunoYAMLTestFile(t, root, name+"/opencollection.yml", "info:\n  name: "+name+"\n")
		writeBrunoYAMLTestFile(t, root, name+"/ping.yml", `
info:
  name: ping
  type: http
http:
  method: GET
  url: https://example.com/ping
`)
	}

	collections, err := ReadBrunoDirectory(root)
	if err != nil {
		t.Fatalf("read bruno yaml directory: %v", err)
	}
	if len(collections) != 2 {
		t.Fatalf("expected two collections, got %d", len(collections))
	}
	if collections[0].Name != "first" || collections[1].Name != "second" {
		t.Errorf("unexpected collections: %+v", collections)
	}
}

func writeBrunoYAMLTestFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()

	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
