package collections

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/awake-client/awake/src/domain"
)

func writeTestRequest(t *testing.T, workspaceRoot string, relativePath string, name string) {
	t.Helper()
	writeTestFile(t, workspaceRoot, relativePath, "name: "+name+"\nmethod: GET\nurl: https://example.com\n")
}

func listedPaths(t *testing.T, repository *Repository, relativePath string) []string {
	t.Helper()
	entries, err := repository.List(relativePath)
	if err != nil {
		t.Fatalf("list %s: %v", relativePath, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	return paths
}

func readTestFile(t *testing.T, workspaceRoot string, relativePath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(workspaceRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(content)
}

func TestArrangePlacesARequestBeforeOrAfterAnother(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "# Shared settings\nname: Platform\n")
	writeTestRequest(t, workspaceRoot, "collections/platform/a.yaml", "A")
	writeTestRequest(t, workspaceRoot, "collections/platform/b.yaml", "B")
	writeTestRequest(t, workspaceRoot, "collections/platform/c.yaml", "C")
	repository := NewRepository(workspaceRoot)

	moved, err := repository.Arrange("collections/platform/c.yaml", "collections/platform/a.yaml", false)
	if err != nil {
		t.Fatalf("arrange: %v", err)
	}
	if moved != "collections/platform/c.yaml" {
		t.Errorf("unexpected path %q", moved)
	}
	want := []string{"collections/platform/c.yaml", "collections/platform/a.yaml", "collections/platform/b.yaml"}
	if got := listedPaths(t, repository, "collections/platform"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order %v", got)
	}

	if _, err := repository.Arrange("collections/platform/c.yaml", "collections/platform/b.yaml", true); err != nil {
		t.Fatalf("arrange after: %v", err)
	}
	want = []string{"collections/platform/a.yaml", "collections/platform/b.yaml", "collections/platform/c.yaml"}
	if got := listedPaths(t, repository, "collections/platform"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order %v", got)
	}

	content := readTestFile(t, workspaceRoot, "collections/platform/collection.yaml")
	wantContent := "# Shared settings\nname: Platform\norder:\n  - a.yaml\n  - b.yaml\n  - c.yaml\n"
	if content != wantContent {
		t.Errorf("unexpected collection file:\n%s", content)
	}
}

func TestArrangeMixesFoldersAndRequests(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\n")
	writeTestRequest(t, workspaceRoot, "collections/platform/health.yaml", "Health")
	writeTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml", "name: Users\n")
	repository := NewRepository(workspaceRoot)

	if _, err := repository.Arrange("collections/platform/health.yaml", "collections/platform/users", false); err != nil {
		t.Fatalf("arrange: %v", err)
	}
	want := []string{"collections/platform/health.yaml", "collections/platform/users"}
	if got := listedPaths(t, repository, "collections/platform"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order %v", got)
	}
}

func TestArrangeMovesARequestNextToAnEntryOfAnotherFolder(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\n")
	writeTestRequest(t, workspaceRoot, "collections/platform/health.yaml", "Health")
	writeTestRequest(t, workspaceRoot, "collections/platform/users/list.yaml", "List")
	writeTestRequest(t, workspaceRoot, "collections/platform/users/show.yaml", "Show")
	repository := NewRepository(workspaceRoot)

	moved, err := repository.Arrange("collections/platform/health.yaml", "collections/platform/users/show.yaml", false)
	if err != nil {
		t.Fatalf("arrange: %v", err)
	}
	if moved != "collections/platform/users/health.yaml" {
		t.Fatalf("unexpected path %q", moved)
	}
	want := []string{
		"collections/platform/users/list.yaml",
		"collections/platform/users/health.yaml",
		"collections/platform/users/show.yaml",
	}
	if got := listedPaths(t, repository, "collections/platform/users"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order %v", got)
	}

	// The folder had no folder.yaml: it receives one named after its directory.
	content := readTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml")
	if content != "name: users\norder:\n  - list.yaml\n  - health.yaml\n  - show.yaml\n" {
		t.Errorf("unexpected folder file:\n%s", content)
	}
}

func TestArrangeRejectsAFolderPlacedInsideItself(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\n")
	writeTestRequest(t, workspaceRoot, "collections/platform/users/admins/list.yaml", "List")

	_, err := NewRepository(workspaceRoot).Arrange("collections/platform/users", "collections/platform/users/admins/list.yaml", false)
	if err != ErrInvalidMove {
		t.Fatalf("expected ErrInvalidMove, got %v", err)
	}
}

func TestArrangeOrdersCollectionsWithinTheirRoot(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/alpha/collection.yaml", "name: Alpha\n")
	writeTestFile(t, workspaceRoot, "collections/beta/collection.yaml", "name: Beta\n")
	writeTestFile(t, workspaceRoot, "local/notes/collection.yaml", "name: Notes\n")
	repository := NewRepository(workspaceRoot)

	if _, err := repository.Arrange("collections/beta", "collections/alpha", false); err != nil {
		t.Fatalf("arrange: %v", err)
	}
	want := []string{"collections/beta", "collections/alpha"}
	if got := listedPaths(t, repository, ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order %v", got)
	}
	if content := readTestFile(t, workspaceRoot, "collections/order.yaml"); content != "order:\n  - beta\n  - alpha\n" {
		t.Errorf("unexpected order file:\n%s", content)
	}

	if _, err := repository.Arrange("collections/beta", "local/notes", false); err != ErrInvalidPath {
		t.Errorf("expected ErrInvalidPath across roots, got %v", err)
	}
	if _, err := repository.Arrange("collections/beta", "collections/alpha/x.yaml", false); err != ErrInvalidPath {
		t.Errorf("expected ErrInvalidPath for a collection placed among requests, got %v", err)
	}

	results, err := repository.SearchRequests("order", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("the order file must not be a request: %+v", results)
	}
	if _, err := repository.LoadRequest("collections/order.yaml"); err != ErrInvalidPath {
		t.Errorf("expected ErrInvalidPath for the order file, got %v", err)
	}
}

func TestOrderedDirectoryListsNewEntriesLast(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\norder:\n  - b.yaml\n  - gone.yaml\n  - a.yaml\n")
	writeTestRequest(t, workspaceRoot, "collections/platform/a.yaml", "A")
	writeTestRequest(t, workspaceRoot, "collections/platform/b.yaml", "B")
	writeTestRequest(t, workspaceRoot, "collections/platform/c.yaml", "C")
	writeTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml", "name: Users\n")

	want := []string{
		"collections/platform/b.yaml",
		"collections/platform/a.yaml",
		"collections/platform/users",
		"collections/platform/c.yaml",
	}
	if got := listedPaths(t, NewRepository(workspaceRoot), "collections/platform"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order %v", got)
	}
}

func TestMoveAndDeleteKeepTheOrderUpToDate(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\norder:\n  - b.yaml\n  - a.yaml\n  - users\n")
	writeTestRequest(t, workspaceRoot, "collections/platform/a.yaml", "A")
	writeTestRequest(t, workspaceRoot, "collections/platform/b.yaml", "B")
	writeTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml", "name: Users\norder:\n  - list.yaml\n")
	writeTestRequest(t, workspaceRoot, "collections/platform/users/list.yaml", "List")
	repository := NewRepository(workspaceRoot)

	if _, err := repository.Move("collections/platform/b.yaml", "collections/platform/users"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if content := readTestFile(t, workspaceRoot, "collections/platform/collection.yaml"); content != "name: Platform\norder:\n  - a.yaml\n  - users\n" {
		t.Errorf("unexpected collection file:\n%s", content)
	}
	if content := readTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml"); content != "name: Users\norder:\n  - list.yaml\n  - b.yaml\n" {
		t.Errorf("unexpected folder file:\n%s", content)
	}

	if err := repository.Delete("collections/platform/users"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if content := readTestFile(t, workspaceRoot, "collections/platform/collection.yaml"); content != "name: Platform\norder:\n  - a.yaml\n" {
		t.Errorf("unexpected collection file after delete:\n%s", content)
	}
}

func TestSavingSettingsKeepsTheOrder(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeTestFile(t, workspaceRoot, "collections/platform/collection.yaml", "name: Platform\norder:\n  - b.yaml\n  - a.yaml\n")
	writeTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml", "name: Users\norder:\n  - z.yaml\n")
	repository := NewRepository(workspaceRoot)

	if err := repository.UpdateCollection("collections/platform", domain.Collection{Name: "Platform API"}); err != nil {
		t.Fatalf("update collection: %v", err)
	}
	if content := readTestFile(t, workspaceRoot, "collections/platform/collection.yaml"); content != "name: Platform API\norder:\n  - b.yaml\n  - a.yaml\n" {
		t.Errorf("unexpected collection file:\n%s", content)
	}

	if err := repository.UpdateCollection("collections/platform/users", domain.Collection{Name: "People"}); err != nil {
		t.Fatalf("update folder: %v", err)
	}
	if content := readTestFile(t, workspaceRoot, "collections/platform/users/folder.yaml"); content != "name: People\norder:\n  - z.yaml\n" {
		t.Errorf("unexpected folder file:\n%s", content)
	}
}
