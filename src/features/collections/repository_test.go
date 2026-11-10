package collections

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
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

func TestCreateFolderInsideCollection(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)

	if err := repository.CreateCollection("collections/billing", domain.Collection{Name: "Billing"}); err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if err := repository.CreateFolder("collections/billing/invoices", "Unpaid invoices"); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := repository.CreateFolder("collections/billing/invoices/drafts", ""); err != nil {
		t.Fatalf("create nested folder: %v", err)
	}

	entries, err := repository.List("collections/billing")
	if err != nil {
		t.Fatalf("list collection: %v", err)
	}
	if len(entries) != 1 || entries[0].Kind != KindDirectory || entries[0].Name != "Unpaid invoices" || !entries[0].HasChildren {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	nested, err := repository.List("collections/billing/invoices/drafts")
	if err != nil {
		t.Fatalf("list nested folder: %v", err)
	}
	if len(nested) != 0 {
		t.Fatalf("the folder metadata must stay hidden, got %+v", nested)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "collections", "billing", "invoices", "drafts", "folder.yaml")); err != nil {
		t.Fatalf("expected a folder metadata file: %v", err)
	}

	if err := repository.CreateFolder("collections/billing/invoices", ""); !errors.Is(err, ErrEntryExists) {
		t.Fatalf("expected ErrEntryExists, got %v", err)
	}
}

func TestCreateFolderRejectsInvalidLocations(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)

	cases := map[string]error{
		"collections/billing":        ErrInvalidPath,
		"collections/../outside/x":   ErrInvalidPath,
		"collections/missing/folder": ErrEntryNotFound,
		"collections/billing/a.yaml": ErrInvalidPath,
	}
	for relativePath, expected := range cases {
		if err := repository.CreateFolder(relativePath, ""); !errors.Is(err, expected) {
			t.Errorf("CreateFolder(%q): expected %v, got %v", relativePath, expected, err)
		}
	}
}

func TestDeleteRemovesRequestsAndFolders(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)
	writeTestFile(t, workspaceRoot, "collections/billing/collection.yaml", "name: Billing\n")
	writeTestFile(t, workspaceRoot, "collections/billing/list.yaml", "name: List\nmethod: GET\nurl: https://example.com\n")
	writeTestFile(t, workspaceRoot, "collections/billing/invoices/get.yaml", "name: Get\nmethod: GET\nurl: https://example.com\n")

	if err := repository.Delete("collections/billing/list.yaml"); err != nil {
		t.Fatalf("delete request: %v", err)
	}
	if err := repository.Delete("collections/billing/invoices"); err != nil {
		t.Fatalf("delete folder: %v", err)
	}

	entries, err := repository.List("collections/billing")
	if err != nil {
		t.Fatalf("list collection: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected an empty collection, got %+v", entries)
	}

	if err := repository.Delete("collections/billing"); err != nil {
		t.Fatalf("delete collection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "collections", "billing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the collection to be gone, got %v", err)
	}
}

func TestDeleteRejectsInvalidTargets(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)
	writeTestFile(t, workspaceRoot, "collections/billing/collection.yaml", "name: Billing\n")

	cases := map[string]error{
		"collections":                         ErrInvalidPath,
		"":                                    ErrInvalidPath,
		"collections/../awake.yaml":           ErrInvalidPath,
		"collections/billing/collection.yaml": ErrInvalidPath,
		"collections/missing.yaml":            ErrEntryNotFound,
	}
	for relativePath, expected := range cases {
		if err := repository.Delete(relativePath); !errors.Is(err, expected) {
			t.Errorf("Delete(%q): expected %v, got %v", relativePath, expected, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "collections", "billing", "collection.yaml")); err != nil {
		t.Fatalf("the collection metadata must survive: %v", err)
	}
}

func TestPrivateCollectionsLiveUnderLocal(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)

	if err := repository.CreateCollection("local/scratch", domain.Collection{Name: "Scratch"}); err != nil {
		t.Fatalf("create private collection: %v", err)
	}
	if err := repository.CreateFolder("local/scratch/drafts", ""); err != nil {
		t.Fatalf("create private folder: %v", err)
	}
	request := domain.Request{Name: "Ping", Method: "GET", URL: "https://example.com/ping"}
	if err := repository.CreateRequest("local/scratch/drafts/ping.yaml", request); err != nil {
		t.Fatalf("create private request: %v", err)
	}

	roots, err := repository.List("local")
	if err != nil {
		t.Fatalf("list private root: %v", err)
	}
	if len(roots) != 1 || roots[0].Kind != KindCollection || roots[0].Name != "Scratch" || roots[0].Path != "local/scratch" {
		t.Fatalf("unexpected private collections: %+v", roots)
	}

	shared, err := repository.List("")
	if err != nil {
		t.Fatalf("list shared root: %v", err)
	}
	if len(shared) != 0 {
		t.Fatalf("private collections must not appear among shared ones: %+v", shared)
	}

	results, err := repository.SearchRequests("ping", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Path != "local/scratch/drafts/ping.yaml" {
		t.Fatalf("expected the private request in search results, got %+v", results)
	}

	if _, err := repository.LoadRequest("local/scratch/drafts/ping.yaml"); err != nil {
		t.Fatalf("load private request: %v", err)
	}
	if err := repository.Delete("local"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("the private root must not be deletable, got %v", err)
	}
	if _, err := repository.List("local/../awake.yaml"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected traversal out of local to be rejected, got %v", err)
	}
}

func TestCollectionEnvironmentsStayOutOfTheTree(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)
	writeTestFile(t, workspaceRoot, "collections/billing/collection.yaml", "name: Billing\n")
	writeTestFile(t, workspaceRoot, "collections/billing/environments/staging.yaml", "name: Staging\n")
	writeTestFile(t, workspaceRoot, "collections/empty/collection.yaml", "name: Empty\n")
	writeTestFile(t, workspaceRoot, "collections/empty/environments/staging.yaml", "name: Staging\n")
	writeTestFile(t, workspaceRoot, "collections/billing/list.yaml", "name: List\nmethod: GET\nurl: https://example.com\n")

	entries, err := repository.List("collections/billing")
	if err != nil {
		t.Fatalf("list collection: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "List" {
		t.Fatalf("expected only the request, got %+v", entries)
	}

	roots, err := repository.List("")
	if err != nil {
		t.Fatalf("list roots: %v", err)
	}
	for _, root := range roots {
		if root.Path == "collections/empty" && root.HasChildren {
			t.Errorf("a collection holding only environments has no children: %+v", root)
		}
	}

	results, err := repository.SearchRequests("staging", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("environment files must not be found as requests: %+v", results)
	}

	if err := repository.CreateFolder("collections/billing/environments", ""); !errors.Is(err, ErrReservedName) {
		t.Fatalf("expected the environments folder name to be reserved, got %v", err)
	}
	if err := repository.CreateFolder("collections/billing/sub", ""); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := repository.CreateFolder("collections/billing/sub/environments", ""); err != nil {
		t.Fatalf("a nested folder may use the name: %v", err)
	}
}

func TestMoveRequestsAndFolders(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)
	request := "name: Ping\nmethod: GET\nurl: https://example.com\n"
	writeTestFile(t, workspaceRoot, "collections/billing/collection.yaml", "name: Billing\n")
	writeTestFile(t, workspaceRoot, "collections/billing/ping.yaml", request)
	writeTestFile(t, workspaceRoot, "collections/billing/invoices/list.yaml", request)
	writeTestFile(t, workspaceRoot, "collections/billing/archive/old.yaml", request)
	writeTestFile(t, workspaceRoot, "local/scratch/collection.yaml", "name: Scratch\n")

	moved, err := repository.Move("collections/billing/ping.yaml", "collections/billing/invoices")
	if err != nil || moved != "collections/billing/invoices/ping.yaml" {
		t.Fatalf("move request into a folder: %q, %v", moved, err)
	}
	moved, err = repository.Move("collections/billing/invoices/ping.yaml", "collections/billing")
	if err != nil || moved != "collections/billing/ping.yaml" {
		t.Fatalf("move request up: %q, %v", moved, err)
	}
	moved, err = repository.Move("collections/billing/archive", "collections/billing/invoices")
	if err != nil || moved != "collections/billing/invoices/archive" {
		t.Fatalf("move folder into a folder: %q, %v", moved, err)
	}
	if _, err := repository.LoadRequest("collections/billing/invoices/archive/old.yaml"); err != nil {
		t.Fatalf("the moved folder keeps its requests: %v", err)
	}
	moved, err = repository.Move("collections/billing/ping.yaml", "local/scratch")
	if err != nil || moved != "local/scratch/ping.yaml" {
		t.Fatalf("make a request private: %q, %v", moved, err)
	}
	if same, err := repository.Move("local/scratch/ping.yaml", "local/scratch"); err != nil || same != "local/scratch/ping.yaml" {
		t.Fatalf("moving into the same folder is a no-op: %q, %v", same, err)
	}

	writeTestFile(t, workspaceRoot, "collections/billing/list.yaml", request)
	cases := []struct {
		source   string
		target   string
		expected error
	}{
		{"collections/billing/invoices", "collections/billing/invoices/archive", ErrInvalidMove},
		{"collections/billing", "local/scratch", ErrInvalidPath},
		{"collections/billing/list.yaml", "collections", ErrInvalidPath},
		{"collections/billing/list.yaml", "collections/billing/invoices", ErrEntryExists},
		{"collections/billing/missing.yaml", "local/scratch", ErrEntryNotFound},
		{"collections/billing/list.yaml", "../outside", ErrInvalidPath},
	}
	for _, testCase := range cases {
		if _, err := repository.Move(testCase.source, testCase.target); !errors.Is(err, testCase.expected) {
			t.Errorf("Move(%q, %q): expected %v, got %v", testCase.source, testCase.target, testCase.expected, err)
		}
	}
}

func TestCollectionsExposeTheirColor(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)
	if err := repository.CreateCollection("collections/billing", domain.Collection{Name: "Billing", Color: "violet"}); err != nil {
		t.Fatalf("create collection: %v", err)
	}
	entries, err := repository.List("")
	if err != nil || len(entries) != 1 || entries[0].Color != "violet" {
		t.Fatalf("expected the collection color, got %+v, %v", entries, err)
	}
	if err := repository.UpdateCollection("collections/billing", domain.Collection{Name: "Billing", Color: "plaid"}); err == nil {
		t.Fatal("expected an unknown color to be rejected")
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

func TestSearchRequestsMatchesNamePathAndMethod(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/users/list.yaml", "name: List users\nmethod: GET\nurl: https://example.com/users\n")
	writeTestFile(t, workspaceRoot, "collections/platform/users/create.yaml", "name: Create user\nmethod: POST\nurl: https://example.com/users\n")
	writeTestFile(t, workspaceRoot, "collections/billing/invoices.yaml", "name: List invoices\nmethod: GET\nurl: https://example.com/invoices\n")
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\n")

	repository := NewRepository(workspaceRoot)

	testCases := []struct {
		query    string
		expected []string
	}{
		{query: "", expected: []string{"Create user", "List invoices", "List users"}},
		{query: "users", expected: []string{"Create user", "List users"}},
		{query: "list users", expected: []string{"List users"}},
		{query: "post", expected: []string{"Create user"}},
		{query: "billing", expected: []string{"List invoices"}},
		{query: "missing", expected: []string{}},
	}

	for _, testCase := range testCases {
		entries, err := repository.SearchRequests(testCase.query, 100)
		if err != nil {
			t.Fatalf("search %q: %v", testCase.query, err)
		}

		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name)
		}

		if strings.Join(names, ",") != strings.Join(testCase.expected, ",") {
			t.Errorf("search %q: expected %v, got %v", testCase.query, testCase.expected, names)
		}
	}
}

func TestSearchRequestsHonoursLimit(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/first.yaml", "name: First\nmethod: GET\nurl: https://example.com/first\n")
	writeTestFile(t, workspaceRoot, "collections/platform/second.yaml", "name: Second\nmethod: GET\nurl: https://example.com/second\n")

	entries, err := NewRepository(workspaceRoot).SearchRequests("", 1)
	if err != nil {
		t.Fatalf("search requests: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestSearchRequestsReturnsNothingWithoutCollections(t *testing.T) {
	entries, err := NewRepository(t.TempDir()).SearchRequests("", 10)
	if err != nil {
		t.Fatalf("search requests: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries, got %+v", entries)
	}
}

func TestRequestOverrideRoundTripAndStaysOutOfTheTree(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/list.yaml", "name: List users\nmethod: GET\nurl: https://example.com\n")
	repository := NewRepository(workspaceRoot)

	parameters := []domain.Parameter{{Name: "page", Value: "2", Enabled: true}}
	override := domain.RequestOverride{
		Parameters: &parameters,
		Body:       &domain.Body{Type: domain.BodyJSON, Content: `{"local":true}`},
	}
	if err := repository.SaveRequestOverride("collections/platform/list.yaml", override); err != nil {
		t.Fatalf("save override: %v", err)
	}

	loaded, err := repository.LoadRequestOverride("collections/platform/list.yaml")
	if err != nil {
		t.Fatalf("load override: %v", err)
	}
	if loaded.Parameters == nil || (*loaded.Parameters)[0].Value != "2" || loaded.Body == nil || loaded.Body.Content != `{"local":true}` {
		t.Errorf("unexpected override: %+v", loaded)
	}

	entries, err := repository.List("collections/platform")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "collections/platform/list.yaml" {
		t.Errorf("the override must not show in the tree: %+v", entries)
	}
	if _, err := repository.LoadRequest("collections/platform/list.local.yaml"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("an override file is not a request, got %v", err)
	}

	if err := repository.SaveRequestOverride("collections/platform/list.yaml", domain.RequestOverride{}); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "collections/platform/list.local.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an empty override must remove the file, got %v", err)
	}
}

func TestEmptyOverrideSectionStillReplaces(t *testing.T) {
	empty := []domain.Parameter{}
	request := domain.Request{Parameters: []domain.Parameter{{Name: "page", Value: "1", Enabled: true}}}

	applied := domain.RequestOverride{Parameters: &empty}.ApplyTo(request)
	if len(applied.Parameters) != 0 {
		t.Errorf("an empty local list must replace the shared one, got %+v", applied.Parameters)
	}
	if kept := (domain.RequestOverride{}).ApplyTo(request); len(kept.Parameters) != 1 {
		t.Errorf("no override must keep the shared parameters, got %+v", kept.Parameters)
	}
}

func TestMoveAndDeleteCarryTheOverride(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/list.yaml", "name: List users\nmethod: GET\nurl: https://example.com\n")
	writeTestFile(t, workspaceRoot, "collections/platform/list.local.yaml", "body:\n  type: text\n  content: local\n")
	writeTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml", "name: Users\n")
	repository := NewRepository(workspaceRoot)

	moved, err := repository.Move("collections/platform/list.yaml", "collections/platform/users")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	override, err := repository.LoadRequestOverride(moved)
	if err != nil || override.Body == nil || override.Body.Content != "local" {
		t.Fatalf("expected the override to follow the request, got %+v (%v)", override, err)
	}

	if err := repository.Delete(moved); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "collections/platform/users/list.local.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected the override to be deleted, got %v", err)
	}
}

func TestResolveInheritedAuthenticationUsesTheClosestAncestor(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/billing/collection.yaml", "name: Billing\nauthentication:\n  type: bearer\n  token: collection-token\n")
	writeTestFile(t, workspaceRoot, "collections/billing/users/folder.yaml", "name: Users\nauthentication:\n  type: basic\n  username: folder-user\n")
	writeTestFile(t, workspaceRoot, "collections/billing/users/admins/folder.yaml", "name: Admins\nauthentication:\n  type: apiKey\n  key: X-Api-Key\n  value: admin-key\n")

	repository := NewRepository(workspaceRoot)
	cases := []struct {
		name       string
		request    string
		expected   domain.Authentication
		sourcePath string
		sourceName string
	}{
		{
			name:       "nested folder wins over the collection",
			request:    "collections/billing/users/admins/list.yaml",
			expected:   domain.Authentication{Type: domain.AuthenticationAPIKey, Key: "X-Api-Key", Value: "admin-key"},
			sourcePath: "collections/billing/users/admins",
			sourceName: "Admins",
		},
		{
			name:       "closest folder falls back to the collection",
			request:    "collections/billing/users/list.yaml",
			expected:   domain.Authentication{Type: domain.AuthenticationBasic, Username: "folder-user"},
			sourcePath: "collections/billing/users",
			sourceName: "Users",
		},
		{
			name:       "request at the collection root uses the collection",
			request:    "collections/billing/list.yaml",
			expected:   domain.Authentication{Type: domain.AuthenticationBearer, Token: "collection-token"},
			sourcePath: "collections/billing",
			sourceName: "Billing",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			inherited, err := repository.ResolveInheritedAuthentication(testCase.request)
			if err != nil {
				t.Fatalf("resolve authentication: %v", err)
			}
			if !reflect.DeepEqual(inherited.Authentication, &testCase.expected) {
				t.Errorf("expected %+v, got %+v", testCase.expected, inherited.Authentication)
			}
			if inherited.SourcePath != testCase.sourcePath || inherited.SourceName != testCase.sourceName {
				t.Errorf("expected source %q (%q), got %q (%q)", testCase.sourcePath, testCase.sourceName, inherited.SourcePath, inherited.SourceName)
			}
		})
	}
}

func TestResolveInheritedAuthenticationForScopeSkipsItself(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/billing/collection.yaml", "name: Billing\nauthentication:\n  type: bearer\n  token: collection-token\n")
	writeTestFile(t, workspaceRoot, "collections/billing/users/folder.yaml", "name: Users\nauthentication:\n  type: basic\n  username: folder-user\n")
	writeTestFile(t, workspaceRoot, "collections/billing/users/admins/folder.yaml", "name: Admins\nauthentication:\n  type: apiKey\n  key: X-Api-Key\n  value: admin-key\n")

	repository := NewRepository(workspaceRoot)
	inherited, err := repository.ResolveInheritedAuthenticationForScope("collections/billing/users/admins")
	if err != nil {
		t.Fatalf("resolve folder authentication: %v", err)
	}
	if inherited.Authentication == nil || inherited.Authentication.Type != domain.AuthenticationBasic || inherited.Authentication.Username != "folder-user" {
		t.Fatalf("expected the parent folder authentication, got %+v", inherited.Authentication)
	}
	if inherited.SourcePath != "collections/billing/users" || inherited.SourceName != "Users" {
		t.Errorf("expected source collections/billing/users (Users), got %q (%q)", inherited.SourcePath, inherited.SourceName)
	}

	if _, err := repository.ResolveInheritedAuthenticationForScope("collections/billing"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("a collection has no parent scope, got %v", err)
	}
}

func TestResolveInheritedAuthenticationWithoutAncestorReturnsNone(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/open/collection.yaml", "name: Open\n")

	inherited, err := NewRepository(workspaceRoot).ResolveInheritedAuthentication("collections/open/ping.yaml")
	if err != nil {
		t.Fatalf("resolve authentication: %v", err)
	}
	if inherited.Authentication == nil || inherited.Authentication.Type != domain.AuthenticationNone {
		t.Fatalf("expected no authentication, got %+v", inherited.Authentication)
	}
	if inherited.SourcePath != "" || inherited.SourceName != "" {
		t.Errorf("expected no source, got %q (%q)", inherited.SourcePath, inherited.SourceName)
	}
}

func TestUpdateCollectionWritesFolderMetadata(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)

	if err := repository.CreateCollection("collections/billing", domain.Collection{Name: "Billing"}); err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if err := repository.CreateFolder("collections/billing/invoices", ""); err != nil {
		t.Fatalf("create folder: %v", err)
	}

	updated := domain.Collection{
		Name:           "Invoices",
		Variables:      map[string]string{"pageSize": "20"},
		Documentation:  "Paid invoices",
		Authentication: &domain.Authentication{Type: domain.AuthenticationBearer, Token: "{{token}}"},
	}
	if err := repository.UpdateCollection("collections/billing/invoices", updated); err != nil {
		t.Fatalf("update folder: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workspaceRoot, "collections", "billing", "invoices", "folder.yaml"))
	if err != nil {
		t.Fatalf("read folder metadata: %v", err)
	}
	for _, expected := range []string{"name: Invoices", "variables:", "authentication:", "type: bearer"} {
		if !strings.Contains(string(content), expected) {
			t.Errorf("expected folder metadata to contain %q, got:\n%s", expected, content)
		}
	}

	loaded, err := repository.LoadCollection("collections/billing/invoices")
	if err != nil {
		t.Fatalf("load folder: %v", err)
	}
	if loaded.Name != "Invoices" || loaded.Variables["pageSize"] != "20" || loaded.Documentation != "Paid invoices" {
		t.Errorf("unexpected folder metadata: %+v", loaded)
	}
	if loaded.Authentication == nil || loaded.Authentication.Type != domain.AuthenticationBearer || loaded.Authentication.Token != "{{token}}" {
		t.Errorf("unexpected folder authentication: %+v", loaded.Authentication)
	}
}

func TestLoadCollectionFallsBackToTheFolderName(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/citadel/badging-dialog/clockings-info/info.yaml", "name: Info\nmethod: GET\nurl: https://example.com\n")
	repository := NewRepository(workspaceRoot)

	loaded, err := repository.LoadCollection("collections/citadel/badging-dialog/clockings-info")
	if err != nil {
		t.Fatalf("load folder without metadata: %v", err)
	}
	if loaded.Name != "clockings-info" {
		t.Errorf("expected the directory name, got %q", loaded.Name)
	}
	if len(loaded.Variables) != 0 || loaded.Authentication != nil || loaded.Documentation != "" {
		t.Errorf("expected an empty configuration, got %+v", loaded)
	}

	if _, err := repository.LoadCollection("collections/citadel/missing"); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("a missing folder must stay not found, got %v", err)
	}
}
