package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/collections"
	"github.com/awake-client/awake/src/features/workspace"
)

func TestCollectionWriterStoresVariablesInTheRequestCollection(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Writer", Mode: workspace.ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	filePath := filepath.Join(root, "collections", "platform", "collection.yaml")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Platform defaults\nname: Platform\nvariables:\n  baseUrl: https://api.example.com\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := collectionWriter{}
	if err := writer.ApplyCollectionChanges(root, "collections/platform/users/list.yaml", []domain.CollectionChange{
		{Name: "cookies", Value: "a=1; b=2"},
		{Name: "token", Value: "s3cret", Local: true},
	}); err != nil {
		t.Fatalf("apply changes: %v", err)
	}

	written, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read collection: %v", err)
	}
	text := string(written)
	if !strings.Contains(text, "# Platform defaults") || !strings.Contains(text, "cookies: a=1; b=2") {
		t.Errorf("expected the variable to be stored next to the comment:\n%s", text)
	}

	localPath := filepath.Join(root, "collections", "platform", "collection.local.yaml")
	local, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read local collection: %v", err)
	}
	if !strings.Contains(string(local), "token: s3cret") {
		t.Errorf("expected the local variable in collection.local.yaml:\n%s", local)
	}

	// A request outside any collection is rejected.
	if err := writer.ApplyCollectionChanges(root, "awake.yaml", []domain.CollectionChange{{Name: "x", Value: "y"}}); !errors.Is(err, collections.ErrInvalidPath) {
		t.Errorf("expected an invalid path error, got %v", err)
	}
}

func TestCollectionWriterReportsMissingCollectionSettings(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Writer", Mode: workspace.ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	writer := collectionWriter{}
	err := writer.ApplyCollectionChanges(root, "collections/other/ping.yaml", []domain.CollectionChange{{Name: "x", Value: "y"}})
	if err == nil || !strings.Contains(err.Error(), "no settings yet") {
		t.Errorf("expected a missing settings error, got %v", err)
	}
}
