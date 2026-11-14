package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
)

func TestApplyEnvironmentChangesWritesPrivateEnvironment(t *testing.T) {
	workspaceRoot := t.TempDir()
	privatePath := filepath.Join(workspaceRoot, "environments", "scratch.local.yaml")
	writeFile(t, privatePath, "name: Scratch\nvariables:\n  baseUrl: http://localhost:3000\n")

	writer := environmentWriter{}
	changes := []domain.EnvironmentChange{{Name: "baseUrl", Value: "http://localhost:4000"}}
	if err := writer.ApplyEnvironmentChanges(workspaceRoot, "collections/billing/invoices.yaml", "scratch", changes); err != nil {
		t.Fatalf("apply environment changes: %v", err)
	}

	local, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("read private environment: %v", err)
	}
	if !strings.Contains(string(local), "http://localhost:4000") {
		t.Errorf("expected the private environment to keep the new value, got:\n%s", local)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "environments", "scratch.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no shared file for a private environment, got %v", err)
	}
}

func TestApplyEnvironmentChangesWritesPrivateEnvironmentWithoutVariables(t *testing.T) {
	workspaceRoot := t.TempDir()
	privatePath := filepath.Join(workspaceRoot, "environments", "scratch.local.yaml")
	writeFile(t, privatePath, "name: Scratch\nsecretVariables:\n  - apiToken\n")

	writer := environmentWriter{}
	changes := []domain.EnvironmentChange{{Name: "lastId", Value: "550"}}
	if err := writer.ApplyEnvironmentChanges(workspaceRoot, "collections/billing/invoices.yaml", "scratch", changes); err != nil {
		t.Fatalf("apply environment changes: %v", err)
	}

	local, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatalf("read private environment: %v", err)
	}
	if !strings.Contains(string(local), "lastId") {
		t.Errorf("expected the private environment to hold the new variable, got:\n%s", local)
	}
}

func TestResolveEnvironmentNameMatchesFileOrDisplayName(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeFile(t, filepath.Join(workspaceRoot, "environments", "production.yaml"), "name: Production\n")
	writeFile(t, filepath.Join(workspaceRoot, "collections", "billing", "environments", "sandbox.yaml"), "name: Billing sandbox\n")

	writer := environmentWriter{}
	requestPath := "collections/billing/invoices.yaml"
	cases := map[string]string{
		"production":      "production",
		"Production":      "production",
		"billing SANDBOX": "sandbox",
	}
	for name, expected := range cases {
		fileName, _, err := writer.ResolveEnvironmentName(workspaceRoot, requestPath, name)
		if err != nil || fileName != expected {
			t.Errorf("%q: expected %q, got %q (%v)", name, expected, fileName, err)
		}
	}

	if _, _, err := writer.ResolveEnvironmentName(workspaceRoot, requestPath, "staging"); err == nil {
		t.Error("expected an unknown environment to fail")
	}
}

func TestResolveEnvironmentNameFindsCollectionEnvironmentOfAFolderRequest(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeFile(t, filepath.Join(workspaceRoot, "collections", "billing", "environments", "sandbox.yaml"), "name: Billing sandbox\n")

	writer := environmentWriter{}
	fileName, _, err := writer.ResolveEnvironmentName(workspaceRoot, "collections/billing/invoices/list.yaml", "sandbox")
	if err != nil || fileName != "sandbox" {
		t.Fatalf("expected the collection environment, got %q (%v)", fileName, err)
	}
}

func TestApplyEnvironmentChangesWritesTheCollectionEnvironmentOfAFolderRequest(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeFile(t, filepath.Join(workspaceRoot, "collections", "billing", "environments", "sandbox.yaml"), "name: Sandbox\nvariables:\n  token: first\n")

	writer := environmentWriter{}
	changes := []domain.EnvironmentChange{{Name: "token", Value: "second"}}
	if err := writer.ApplyEnvironmentChanges(workspaceRoot, "collections/billing/invoices/list.yaml", "sandbox", changes); err != nil {
		t.Fatalf("apply environment changes: %v", err)
	}

	local, err := os.ReadFile(filepath.Join(workspaceRoot, "collections", "billing", "environments", "sandbox.local.yaml"))
	if err != nil {
		t.Fatalf("read local override: %v", err)
	}
	if !strings.Contains(string(local), "second") {
		t.Errorf("expected the collection environment to keep the new value, got:\n%s", local)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
