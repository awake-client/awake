package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/features/workspace"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPServerListsSendsAndWritesRequests(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"path":"` + request.URL.Path + `","padding":"` + strings.Repeat("x", 2000) + `"}`))
	}))
	defer api.Close()

	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Agents", Mode: workspace.ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	write := func(path string, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("collections/billing/collection.yaml", "name: Billing\n")
	write("collections/billing/environments/staging.yaml", "name: Staging\nvariables:\n  baseUrl: "+api.URL+"\n")
	write("collections/billing/invoices/list.yaml", "name: List invoices\nmethod: GET\nurl: \"{{baseUrl}}/invoices\"\n")

	t.Chdir(filepath.Join(root, "collections"))
	found, err := findWorkspaceRoot("")
	if err != nil || found != root {
		t.Fatalf("expected the workspace to be found from a subdirectory, got %q, %v", found, err)
	}
	configuration, err := workspace.NewRepository(root).Load()
	if err != nil {
		t.Fatal(err)
	}
	services, err := newServices()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	if _, err := newMCPServer(root, configuration, services, false).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	call := func(name string, arguments map[string]any) (bool, map[string]any) {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: arguments})
		if err != nil {
			t.Fatalf("call %s: %v", name, err)
		}
		output := map[string]any{}
		if result.StructuredContent != nil {
			content, _ := json.Marshal(result.StructuredContent)
			_ = json.Unmarshal(content, &output)
		}
		return result.IsError, output
	}

	_, listed := call("list_requests", map[string]any{})
	paths := []string{}
	for _, entry := range listed["entries"].([]any) {
		paths = append(paths, entry.(map[string]any)["path"].(string))
	}
	joined := strings.Join(paths, ",")
	if !strings.Contains(joined, "collections/billing/invoices/list.yaml") || !strings.Contains(joined, "collections/billing/invoices") {
		t.Fatalf("expected the tree with folders and requests, got %s", joined)
	}

	failed, sent := call("send_request", map[string]any{
		"path":         "collections/billing/invoices/list.yaml",
		"environment":  "staging",
		"maxBodyBytes": 100,
	})
	if failed || sent["statusCode"].(float64) != 200 || !strings.Contains(sent["body"].(string), `"path":"/invoices"`) {
		t.Fatalf("unexpected response: %v", sent)
	}
	if sent["bodyTruncated"] != true || len(sent["body"].(string)) > 100 {
		t.Errorf("expected the body to be cut at 100 bytes, got %d bytes", len(sent["body"].(string)))
	}

	_, history := call("list_history", map[string]any{"requestPath": "collections/billing/invoices/list.yaml"})
	if entries, _ := history["entries"].([]any); len(entries) != 1 {
		t.Errorf("expected the send in the history, got %v", history)
	}

	failed, _ = call("update_request", map[string]any{
		"path":    "collections/billing/invoices/missing.yaml",
		"request": map[string]any{"name": "Missing", "method": "GET", "url": "https://example.com"},
	})
	if !failed {
		t.Error("expected updating a missing request to fail")
	}

	failed, created := call("create_request", map[string]any{
		"path":    "collections/billing/invoices/create.yaml",
		"request": map[string]any{"name": "Create invoice", "method": "POST", "url": "{{baseUrl}}/invoices"},
	})
	if failed || created["path"] != "collections/billing/invoices/create.yaml" {
		t.Fatalf("unexpected creation: %v", created)
	}
	failed, read := call("get_request", map[string]any{"path": "collections/billing/invoices/create.yaml"})
	if request, _ := read["request"].(map[string]any); failed || request["method"] != "POST" {
		t.Fatalf("expected to read the created request, got %v", read)
	}

	failed, collection := call("create_collection", map[string]any{"name": "Équipe Users", "private": true})
	if failed || collection["path"] != "local/equipe-users" {
		t.Errorf("unexpected collection: %v", collection)
	}
}
