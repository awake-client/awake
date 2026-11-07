package app

import (
	"os"
	"path/filepath"
	"testing"
)

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

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
