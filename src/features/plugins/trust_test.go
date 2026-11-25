package plugins

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testWorkspaceID = "workspace-1"

func TestTrustStoreRequiresTheApprovedCommitAndPermissions(t *testing.T) {
	store := newTestTrustStore(t)
	approval := Approval{
		WorkspaceID: testWorkspaceID,
		PluginID:    "aws-sigv4",
		Repository:  testRepositoryURL,
		Commit:      firstCommit,
		Permissions: []Permission{PermissionSecrets, PermissionNetwork},
	}
	if err := store.Approve(approval); err != nil {
		t.Fatalf("approve: %v", err)
	}

	cases := []struct {
		name        string
		workspaceID string
		pluginID    string
		commit      string
		permissions []Permission
		expected    bool
	}{
		{"approved", testWorkspaceID, "aws-sigv4", firstCommit, []Permission{PermissionNetwork, PermissionSecrets}, true},
		{"fewer permissions", testWorkspaceID, "aws-sigv4", firstCommit, []Permission{PermissionNetwork}, true},
		{"new permission", testWorkspaceID, "aws-sigv4", firstCommit, []Permission{PermissionNetwork, PermissionWorkspaceWrite}, false},
		{"new commit", testWorkspaceID, "aws-sigv4", secondCommit, []Permission{PermissionNetwork}, false},
		{"other workspace", "workspace-2", "aws-sigv4", firstCommit, nil, false},
		{"other plugin", testWorkspaceID, "insomnia", firstCommit, nil, false},
	}
	for _, test := range cases {
		trusted, err := store.IsTrusted(test.workspaceID, test.pluginID, test.commit, test.permissions)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if trusted != test.expected {
			t.Errorf("%s: expected %v, got %v", test.name, test.expected, trusted)
		}
	}
}

func TestTrustStoreReplacesAndRevokesApprovals(t *testing.T) {
	store := newTestTrustStore(t)
	first := Approval{WorkspaceID: testWorkspaceID, PluginID: "aws-sigv4", Commit: firstCommit}
	second := Approval{WorkspaceID: testWorkspaceID, PluginID: "aws-sigv4", Commit: secondCommit}
	other := Approval{WorkspaceID: testWorkspaceID, PluginID: "insomnia", Commit: firstCommit}
	for _, approval := range []Approval{first, other, second} {
		if err := store.Approve(approval); err != nil {
			t.Fatalf("approve: %v", err)
		}
	}

	approvals, err := store.Approvals(testWorkspaceID)
	if err != nil {
		t.Fatalf("approvals: %v", err)
	}
	if len(approvals) != 2 {
		t.Fatalf("expected the second approval to replace the first, got %+v", approvals)
	}
	if trusted, _ := store.IsTrusted(testWorkspaceID, "aws-sigv4", firstCommit, nil); trusted {
		t.Error("the replaced commit must no longer be trusted")
	}

	if err := store.Revoke(testWorkspaceID, "aws-sigv4"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if trusted, _ := store.IsTrusted(testWorkspaceID, "aws-sigv4", secondCommit, nil); trusted {
		t.Error("a revoked plugin must not be trusted")
	}
	if trusted, _ := store.IsTrusted(testWorkspaceID, "insomnia", firstCommit, nil); !trusted {
		t.Error("revoking one plugin must keep the others")
	}
	if err := store.Revoke(testWorkspaceID, "unknown"); err != nil {
		t.Errorf("revoking an unknown plugin must succeed: %v", err)
	}
}

func TestTrustStorePersistsApprovals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", trustFileName)
	store := NewTrustStore(path)
	store.now = func() time.Time { return time.Date(2026, 11, 25, 10, 30, 0, 0, time.UTC) }
	approval := Approval{
		WorkspaceID: testWorkspaceID,
		PluginID:    "aws-sigv4",
		Commit:      firstCommit,
		Permissions: []Permission{PermissionNetwork, PermissionNetwork, PermissionClipboard},
	}
	if err := store.Approve(approval); err != nil {
		t.Fatalf("approve: %v", err)
	}

	reopened := NewTrustStore(path)
	approvals, err := reopened.Approvals(testWorkspaceID)
	if err != nil {
		t.Fatalf("approvals: %v", err)
	}
	if len(approvals) != 1 {
		t.Fatalf("expected one approval, got %+v", approvals)
	}
	saved := approvals[0]
	if strings.Join(permissionNames(saved.Permissions), ",") != "clipboard,network" {
		t.Errorf("expected sorted, unique permissions, got %v", saved.Permissions)
	}
	if !saved.ApprovedAt.Equal(time.Date(2026, 11, 25, 10, 30, 0, 0, time.UTC)) {
		t.Errorf("unexpected approval time: %v", saved.ApprovedAt)
	}
	content, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(content), "version: 1\n") {
		t.Errorf("unexpected file content %q: %v", content, err)
	}
}

func TestTrustStoreRefusesIncompleteApprovals(t *testing.T) {
	store := newTestTrustStore(t)

	if err := store.Approve(Approval{PluginID: "aws-sigv4", Commit: firstCommit}); !errors.Is(err, ErrIncompleteApproval) {
		t.Errorf("expected ErrIncompleteApproval, got %v", err)
	}
	if err := store.Approve(Approval{WorkspaceID: testWorkspaceID, PluginID: "aws-sigv4", Commit: "main"}); !errors.Is(err, ErrInvalidCommit) {
		t.Errorf("expected ErrInvalidCommit, got %v", err)
	}
}

func TestTrustStoreRefusesAFileFromANewerAwake(t *testing.T) {
	path := filepath.Join(t.TempDir(), trustFileName)
	if err := os.WriteFile(path, []byte("version: 2\napprovals: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewTrustStore(path).IsTrusted(testWorkspaceID, "aws-sigv4", firstCommit, nil)

	if err == nil || !strings.Contains(err.Error(), "newer Awake") {
		t.Fatalf("expected a newer version error, got %v", err)
	}
}

func newTestTrustStore(t *testing.T) *TrustStore {
	t.Helper()
	return NewTrustStore(filepath.Join(t.TempDir(), trustFileName))
}

func permissionNames(permissions []Permission) []string {
	names := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		names = append(names, string(permission))
	}
	return names
}
