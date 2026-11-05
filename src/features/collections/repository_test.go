package collections

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clementherve/awake/src/domain"
)

func TestListRootReturnsCollections(t *testing.T) {
	workspaceRoot := t.TempDir()

	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform API\n")
	writeTestFile(t, workspaceRoot, "collections/platform/users/list.yaml", "name: List users\nmethod: GET\nurl: https://example.com/users\n")
	writeTestFile(t, workspaceRoot, "collections/empty/notes.md", "ignored")

	entries, err := NewRepository(workspaceRoot).List("")
	if err != nil {
		t.Fatalf("list root: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 collections, got %d: %+v", len(entries), entries)
	}
	if entries[0].Name != "empty" || entries[0].Kind != KindCollection {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Name != "Platform API" || entries[1].Kind != KindCollection {
		t.Errorf("unexpected second entry: %+v", entries[1])
	}
	if !entries[1].HasChildren {
		t.Error("expected Platform API to have children")
	}
	if entries[1].Path != "collections/platform" {
		t.Errorf("unexpected path %q", entries[1].Path)
	}
}

func TestListSortsFoldersBeforeRequests(t *testing.T) {
	workspaceRoot := t.TempDir()

	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\n")
	writeTestFile(t, workspaceRoot, "collections/platform/zebra.yaml", "name: Zebra\nmethod: GET\nurl: https://example.com/zebra\n")
	writeTestFile(t, workspaceRoot, "collections/platform/users/list.yaml", "name: List users\nmethod: GET\nurl: https://example.com/users\n")

	entries, err := NewRepository(workspaceRoot).List("collections/platform")
	if err != nil {
		t.Fatalf("list collection: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Kind != KindDirectory || entries[0].Name != "users" {
		t.Errorf("expected users directory first, got %+v", entries[0])
	}
	if entries[1].Kind != KindRequest || entries[1].Name != "Zebra" || entries[1].Method != "GET" {
		t.Errorf("unexpected request entry: %+v", entries[1])
	}
}

func TestListSkipsCollectionMetadataFile(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\n")

	entries, err := NewRepository(workspaceRoot).List("collections/platform")
	if err != nil {
		t.Fatalf("list collection: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %+v", entries)
	}
}

func TestLoadRequestAppliesDefaults(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/users/list.yaml", `
name: List users
method: get
url: "{{baseUrl}}/users"
`)

	request, err := NewRepository(workspaceRoot).LoadRequest("collections/platform/users/list.yaml")
	if err != nil {
		t.Fatalf("load request: %v", err)
	}

	if request.Method != domain.MethodGet {
		t.Errorf("expected GET, got %q", request.Method)
	}
	if request.Authentication == nil || request.Authentication.Type != domain.AuthenticationInherit {
		t.Errorf("expected inherited authentication, got %+v", request.Authentication)
	}
	if request.Body == nil || request.Body.Type != domain.BodyNone {
		t.Errorf("expected no body, got %+v", request.Body)
	}
}

func TestLoadRequestRejectsInvalidMethod(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/users/list.yaml", "name: List users\nmethod: FETCH\nurl: https://example.com\n")

	_, err := NewRepository(workspaceRoot).LoadRequest("collections/platform/users/list.yaml")
	if !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestCreateRequestWritesCanonicalYaml(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\n")

	request := domain.Request{
		Name:   "Create user",
		Method: domain.MethodPost,
		URL:    "{{baseUrl}}/users",
		Parameters: []domain.Parameter{
			{Name: "notify", Value: "true", Enabled: true},
		},
		Headers: []domain.Header{
			{Name: "Content-Type", Value: "application/json", Enabled: true},
		},
	}

	repository := NewRepository(workspaceRoot)
	relativePath := "collections/platform/users/create-user.yaml"
	if err := repository.CreateRequest(relativePath, request); err != nil {
		t.Fatalf("create request: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workspaceRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read created request: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "- {name: notify, value: \"true\", enabled: true}") {
		t.Errorf("expected compact parameter entry:\n%s", text)
	}
	if !strings.Contains(text, "authentication:\n  type: inherit") {
		t.Errorf("expected default authentication:\n%s", text)
	}
	if !strings.Contains(text, "body:\n  type: none") {
		t.Errorf("expected default body:\n%s", text)
	}

	if err := repository.CreateRequest(relativePath, request); !errors.Is(err, ErrEntryExists) {
		t.Fatalf("expected ErrEntryExists, got %v", err)
	}

	loaded, err := repository.LoadRequest(relativePath)
	if err != nil {
		t.Fatalf("load created request: %v", err)
	}
	if loaded.Name != "Create user" || len(loaded.Parameters) != 1 {
		t.Errorf("unexpected loaded request: %+v", loaded)
	}
}

func TestCreateCollectionWritesMetadata(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)

	collection := domain.Collection{Name: "Billing"}
	if err := repository.CreateCollection("collections/billing", collection); err != nil {
		t.Fatalf("create collection: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workspaceRoot, "collections", "billing", "collection.yaml"))
	if err != nil {
		t.Fatalf("read collection metadata: %v", err)
	}
	if !strings.Contains(string(content), "name: Billing") {
		t.Errorf("unexpected collection metadata:\n%s", content)
	}

	if err := repository.CreateCollection("collections/billing", collection); !errors.Is(err, ErrEntryExists) {
		t.Fatalf("expected ErrEntryExists, got %v", err)
	}
}

func TestListRejectsPathTraversal(t *testing.T) {
	repository := NewRepository(t.TempDir())

	for _, relativePath := range []string{"../outside", "/etc", "collections/../../outside"} {
		if _, err := repository.List(relativePath); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("expected ErrInvalidPath for %q, got %v", relativePath, err)
		}
	}
}

func writeTestFile(t *testing.T, workspaceRoot string, relativePath string, content string) {
	t.Helper()

	filePath := filepath.Join(workspaceRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", relativePath, err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}

func TestUpdateRequestRewritesExistingFile(t *testing.T) {
	workspaceRoot := t.TempDir()
	relativePath := "collections/platform/users/list.yaml"
	writeTestFile(t, workspaceRoot, relativePath, "name: List users\nmethod: GET\nurl: https://example.com/users\n")

	repository := NewRepository(workspaceRoot)
	request := domain.Request{
		Name:   "List active users",
		Method: domain.MethodGet,
		URL:    "{{baseUrl}}/users?active=true",
	}
	if err := repository.UpdateRequest(relativePath, request); err != nil {
		t.Fatalf("update request: %v", err)
	}

	loaded, err := repository.LoadRequest(relativePath)
	if err != nil {
		t.Fatalf("load updated request: %v", err)
	}
	if loaded.Name != "List active users" || loaded.URL != "{{baseUrl}}/users?active=true" {
		t.Errorf("unexpected loaded request: %+v", loaded)
	}
}

func TestUpdateRequestRejectsMissingFile(t *testing.T) {
	err := NewRepository(t.TempDir()).UpdateRequest("collections/platform/missing.yaml", domain.Request{
		Name:   "Missing",
		Method: domain.MethodGet,
		URL:    "https://example.com",
	})
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
}
