package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateWritesEditorSchemas(t *testing.T) {
	root := t.TempDir()
	if _, err := Create(root, CreateOptions{Name: "Schemas", Mode: ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	settings := readSettings(t, root)
	associations, ok := settings[yamlSchemasSetting].(map[string]any)
	if !ok || len(associations) != len(editorSchemas()) {
		t.Fatalf("unexpected associations: %v", settings)
	}
	for _, schema := range editorSchemas() {
		content, err := os.ReadFile(filepath.Join(root, ".vscode", "awake", schema.fileName))
		if err != nil {
			t.Fatalf("read %s: %v", schema.fileName, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(content, &decoded); err != nil || decoded["$schema"] != jsonSchemaDraft {
			t.Errorf("%s is not a JSON schema: %v", schema.fileName, err)
		}
		if _, found := associations[".vscode/awake/"+schema.fileName]; !found {
			t.Errorf("%s is not associated", schema.fileName)
		}
	}
}

func TestEditorSchemasKeepOtherSettings(t *testing.T) {
	root := t.TempDir()
	writeSettings(t, root, `{"editor.tabSize": 2, "yaml.schemas": {"other.json": ["x.yaml"]}}`)

	if err := EnsureEditorSchemas(root); err != nil {
		t.Fatalf("ensure schemas: %v", err)
	}

	settings := readSettings(t, root)
	if settings["editor.tabSize"] != float64(2) {
		t.Errorf("expected other settings to be kept, got %v", settings)
	}
	associations := settings[yamlSchemasSetting].(map[string]any)
	if _, found := associations["other.json"]; !found {
		t.Errorf("expected other schema associations to be kept, got %v", associations)
	}
}

func TestEditorSchemasLeaveCommentedSettingsAlone(t *testing.T) {
	root := t.TempDir()
	commented := "{\n  // mine\n  \"editor.tabSize\": 2,\n}\n"
	writeSettings(t, root, commented)

	if err := EnsureEditorSchemas(root); err != nil {
		t.Fatalf("ensure schemas: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(root, ".vscode", "settings.json"))
	if string(content) != commented {
		t.Errorf("a JSONC settings file must not be rewritten, got:\n%s", content)
	}
}

func TestEditorSchemasAreOnlyWrittenWhenChanged(t *testing.T) {
	root := t.TempDir()
	if err := EnsureEditorSchemas(root); err != nil {
		t.Fatalf("ensure schemas: %v", err)
	}
	settingsPath := filepath.Join(root, ".vscode", "settings.json")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(settingsPath, past, past); err != nil {
		t.Fatal(err)
	}

	if err := EnsureEditorSchemas(root); err != nil {
		t.Fatalf("ensure schemas again: %v", err)
	}
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Error("unchanged settings must not be rewritten")
	}
}

func writeSettings(t *testing.T, root string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".vscode", "settings.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readSettings(t *testing.T, root string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, ".vscode", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	settings := map[string]any{}
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return settings
}
