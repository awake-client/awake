package environments

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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

	interpolated, err := Interpolate(
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
