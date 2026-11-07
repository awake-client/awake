package environments

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/shared/variables"
)

func TestListReturnsSharedEnvironmentsSorted(t *testing.T) {
	workspaceRoot := t.TempDir()

	writeEnvironmentFile(t, workspaceRoot, "staging.yaml", "name: Staging\nvariables:\n  baseUrl: https://staging.example.com\n")
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", "name: Development\nvariables:\n  baseUrl: https://development.example.com\n")
	writeEnvironmentFile(t, workspaceRoot, "development.local.yaml", "name: Development\nvariables:\n  pageSize: \"50\"\n")
	writeEnvironmentFile(t, workspaceRoot, "notes.txt", "ignored")

	summaries, err := NewRepository(workspaceRoot).List()
	if err != nil {
		t.Fatalf("list environments: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 environments, got %d: %+v", len(summaries), summaries)
	}
	if summaries[0].Name != "Development" || summaries[0].FileName != "development" {
		t.Errorf("unexpected first environment: %+v", summaries[0])
	}
	if !summaries[0].HasLocalOverrides {
		t.Error("expected development to report local overrides")
	}
	if summaries[1].Name != "Staging" || summaries[1].HasLocalOverrides {
		t.Errorf("unexpected second environment: %+v", summaries[1])
	}
}

func TestLoadMergesLocalOverrides(t *testing.T) {
	workspaceRoot := t.TempDir()

	writeEnvironmentFile(t, workspaceRoot, "development.yaml", `
name: Development
variables:
  baseUrl: https://development.example.com
  pageSize: "20"
secretVariables:
  - apiToken
`)
	writeEnvironmentFile(t, workspaceRoot, "development.local.yaml", `
variables:
  pageSize: "50"
  apiToken: local-token-value
`)

	resolved, err := NewRepository(workspaceRoot).Load("development")
	if err != nil {
		t.Fatalf("load environment: %v", err)
	}

	if resolved.Local == nil {
		t.Fatal("expected local overrides")
	}
	if resolved.Name != "Development" {
		t.Errorf("expected Development, got %q", resolved.Name)
	}

	interpolated, err := variables.Interpolate(
		"{{baseUrl}}?pageSize={{pageSize}}",
		resolved.Layers()...,
	)
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if interpolated != "https://development.example.com?pageSize=50" {
		t.Errorf("unexpected interpolation %q", interpolated)
	}

	secretNames := resolved.SecretNames()
	if len(secretNames) != 1 || secretNames[0] != "apiToken" {
		t.Errorf("unexpected secret names: %v", secretNames)
	}
}

func TestLoadWithoutLocalOverrides(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", "name: Development\n")

	resolved, err := NewRepository(workspaceRoot).Load("development")
	if err != nil {
		t.Fatalf("load environment: %v", err)
	}
	if resolved.Local != nil {
		t.Errorf("expected no local overrides, got %+v", resolved.Local)
	}
}

func TestLoadRejectsUnknownEnvironment(t *testing.T) {
	_, err := NewRepository(t.TempDir()).Load("missing")
	if !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("expected ErrEnvironmentNotFound, got %v", err)
	}
}

func TestLoadRejectsPathTraversal(t *testing.T) {
	_, err := NewRepository(t.TempDir()).Load("../secrets")
	if !errors.Is(err, ErrInvalidEnvironment) {
		t.Fatalf("expected ErrInvalidEnvironment, got %v", err)
	}
}

func TestLoadRejectsSecretWithPlainValue(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", `
name: Development
variables:
  apiToken: leaked-value
secretVariables:
  - apiToken
`)

	_, err := NewRepository(workspaceRoot).Load("development")
	if !errors.Is(err, ErrInvalidEnvironment) {
		t.Fatalf("expected ErrInvalidEnvironment, got %v", err)
	}
}

func TestListReturnsEmptySliceWithoutDirectory(t *testing.T) {
	summaries, err := NewRepository(t.TempDir()).List()
	if err != nil {
		t.Fatalf("list environments: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected no environments, got %+v", summaries)
	}
}

func writeEnvironmentFile(t *testing.T, workspaceRoot string, fileName string, content string) {
	t.Helper()

	directory := filepath.Join(workspaceRoot, directoryName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create environments directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, fileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", fileName, err)
	}
}

func TestSaveWritesEnvironmentAndKeepsTopLevelComments(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", "# Shared configuration\nname: Development\nvariables:\n  baseUrl: https://old.example.com\nsecretVariables:\n  - apiToken\n")

	repository := NewRepository(workspaceRoot)
	err := repository.Save("development", Environment{
		Name:      "Development",
		Variables: map[string]string{"baseUrl": "https://new.example.com", "pageSize": "50"},
		SecretVariables: []string{
			"apiToken",
			"webhookSecret",
		},
	})
	if err != nil {
		t.Fatalf("save environment: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workspaceRoot, directoryName, "development.yaml"))
	if err != nil {
		t.Fatalf("read environment: %v", err)
	}
	if !strings.Contains(string(content), "# Shared configuration") {
		t.Errorf("expected the comment to be preserved, got:\n%s", content)
	}

	loaded, err := repository.Load("development")
	if err != nil {
		t.Fatalf("load saved environment: %v", err)
	}
	if loaded.Shared.Variables["baseUrl"] != "https://new.example.com" {
		t.Errorf("unexpected variables: %+v", loaded.Shared.Variables)
	}
	if loaded.Shared.Variables["pageSize"] != "50" {
		t.Errorf("unexpected variables: %+v", loaded.Shared.Variables)
	}
	if len(loaded.Shared.SecretVariables) != 2 || loaded.Shared.SecretVariables[1] != "webhookSecret" {
		t.Errorf("unexpected secret variables: %+v", loaded.Shared.SecretVariables)
	}
}

func TestSaveRemovesEmptySections(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", "name: Development\nvariables:\n  baseUrl: https://example.com\n")

	repository := NewRepository(workspaceRoot)
	if err := repository.Save("development", Environment{Name: "Development"}); err != nil {
		t.Fatalf("save environment: %v", err)
	}

	content, err := os.ReadFile(repository.SharedPath("development"))
	if err != nil {
		t.Fatalf("read environment: %v", err)
	}
	if strings.Contains(string(content), "variables") {
		t.Errorf("expected the empty variables section to be removed, got:\n%s", content)
	}
}

func TestDeleteRemovesSharedAndLocalFiles(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeEnvironmentFile(t, workspaceRoot, "staging.yaml", "name: Staging\n")
	writeEnvironmentFile(t, workspaceRoot, "staging.local.yaml", "variables:\n  token: x\n")
	writeEnvironmentFile(t, workspaceRoot, "production.yaml", "name: Production\n")

	repository := NewRepository(workspaceRoot)
	if err := repository.Delete("staging"); err != nil {
		t.Fatalf("delete environment: %v", err)
	}

	summaries, err := repository.List()
	if err != nil {
		t.Fatalf("list environments: %v", err)
	}
	if len(summaries) != 1 || summaries[0].FileName != "production" {
		t.Fatalf("unexpected environments after delete: %+v", summaries)
	}
	if _, err := os.Stat(repository.LocalPath("staging")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected the local overrides to be removed, got %v", err)
	}
	if err := repository.Delete("staging"); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Fatalf("expected ErrEnvironmentNotFound, got %v", err)
	}
	if err := repository.Delete("../awake"); err == nil {
		t.Fatal("expected an invalid file name to be rejected")
	}
}

func TestSaveKeepsTheEnvironmentColor(t *testing.T) {
	workspaceRoot := t.TempDir()
	repository := NewRepository(workspaceRoot)

	if err := repository.Save("production", Environment{Name: "Production", Color: "red"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	summaries, err := repository.List()
	if err != nil || len(summaries) != 1 || summaries[0].Color != "red" {
		t.Fatalf("expected the color in the summary, got %+v, %v", summaries, err)
	}
	if err := repository.Save("production", Environment{Name: "Production"}); err != nil {
		t.Fatalf("save without color: %v", err)
	}
	content, err := os.ReadFile(repository.SharedPath("production"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "color") {
		t.Errorf("clearing the color must remove the key:\n%s", content)
	}
	if err := repository.Save("production", Environment{Name: "Production", Color: "neon"}); err == nil {
		t.Error("expected an unknown color to be rejected")
	}
}

func TestApplyLocalChangesKeepsSharedFileAndComments(t *testing.T) {
	workspaceRoot := t.TempDir()
	shared := "name: Development\nvariables:\n  baseUrl: https://development.example.com\n"
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", shared)
	writeEnvironmentFile(t, workspaceRoot, "development.local.yaml", "# my machine\nvariables:\n  pageSize: \"50\"\n  stale: x\n")

	repository := NewRepository(workspaceRoot)
	err := repository.ApplyLocalChanges("development", []domain.EnvironmentChange{
		{Name: "token", Value: "abc"},
		{Name: "stale", Removed: true},
	})
	if err != nil {
		t.Fatalf("apply local changes: %v", err)
	}

	resolved, err := repository.Load("development")
	if err != nil {
		t.Fatalf("load environment: %v", err)
	}
	if resolved.Local == nil || resolved.Local.Variables["token"] != "abc" || resolved.Local.Variables["pageSize"] != "50" {
		t.Fatalf("unexpected local overrides: %+v", resolved.Local)
	}
	if _, found := resolved.Local.Variables["stale"]; found {
		t.Error("expected stale to be removed")
	}

	sharedContent, _ := os.ReadFile(repository.SharedPath("development"))
	if string(sharedContent) != shared {
		t.Errorf("shared file changed:\n%s", sharedContent)
	}
	localContent, _ := os.ReadFile(repository.LocalPath("development"))
	if !strings.Contains(string(localContent), "# my machine") {
		t.Errorf("expected the comment to survive:\n%s", localContent)
	}
}

func TestApplyLocalChangesCreatesTheLocalFile(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", "name: Development\n")

	repository := NewRepository(workspaceRoot)
	if err := repository.ApplyLocalChanges("development", []domain.EnvironmentChange{{Name: "token", Value: "abc"}}); err != nil {
		t.Fatalf("apply local changes: %v", err)
	}

	resolved, err := repository.Load("development")
	if err != nil {
		t.Fatalf("load environment: %v", err)
	}
	if resolved.Local == nil || resolved.Local.Variables["token"] != "abc" {
		t.Fatalf("unexpected local overrides: %+v", resolved.Local)
	}
}

func TestApplyLocalChangesRejectsInvalidNames(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeEnvironmentFile(t, workspaceRoot, "development.yaml", "name: Development\n")

	err := NewRepository(workspaceRoot).ApplyLocalChanges("development", []domain.EnvironmentChange{{Name: "not valid", Value: "x"}})
	if !errors.Is(err, ErrInvalidEnvironment) {
		t.Fatalf("expected ErrInvalidEnvironment, got %v", err)
	}
}
