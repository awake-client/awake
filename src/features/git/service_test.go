package git

import "testing"

func TestResolveWithinRootRejectsPathTraversal(t *testing.T) {
	for _, path := range []string{"../outside.yaml", "collections/../../outside.yaml"} {
		if _, err := resolveWithinRoot(t.TempDir(), path); err == nil {
			t.Errorf("expected an error for %q", path)
		}
	}
}

func TestResolveWithinRootAcceptsWorkspacePaths(t *testing.T) {
	workspaceRoot := t.TempDir()

	resolved, err := resolveWithinRoot(workspaceRoot, "collections/platform/users/list.yaml")
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	if resolved == "" {
		t.Fatal("expected a resolved path")
	}
}

func TestCommitRequiresMessage(t *testing.T) {
	service := NewService()

	if _, err := service.Commit(t.TempDir(), "   "); err == nil {
		t.Fatal("expected an error for an empty message")
	}
}
