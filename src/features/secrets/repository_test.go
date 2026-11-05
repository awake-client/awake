package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/clementherve/awake/src/features/workspace"
)

type fakeKeychain struct {
	values map[string]string
}

func newFakeKeychain() *fakeKeychain {
	return &fakeKeychain{values: map[string]string{}}
}

func (keychain *fakeKeychain) Get(service string, account string) (string, bool, error) {
	value, found := keychain.values[service+"\x00"+account]
	return value, found, nil
}

func (keychain *fakeKeychain) Set(service string, account string, value string) error {
	keychain.values[service+"\x00"+account] = value
	return nil
}

func (keychain *fakeKeychain) Delete(service string, account string) error {
	delete(keychain.values, service+"\x00"+account)
	return nil
}

func TestServiceStoresAndReportsSecrets(t *testing.T) {
	workspaceRoot := createTestWorkspace(t)
	keychain := newFakeKeychain()
	repository := NewRepository(keychain)

	if err := repository.Set(workspaceRoot, "development", "apiToken", "secret-value"); err != nil {
		t.Fatalf("set secret: %v", err)
	}

	states, err := repository.States(workspaceRoot, "development")
	if err != nil {
		t.Fatalf("secret states: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected two secrets, got %+v", states)
	}
	if !states[0].IsSet || states[0].Name != "apiToken" {
		t.Errorf("unexpected first secret: %+v", states[0])
	}
	if states[1].IsSet || states[1].Name != "webhookSecret" {
		t.Errorf("unexpected second secret: %+v", states[1])
	}

	values, err := repository.Values(workspaceRoot, "development")
	if err != nil {
		t.Fatalf("secret values: %v", err)
	}
	if values["apiToken"] != "secret-value" {
		t.Errorf("unexpected values: %v", values)
	}
	if _, found := values["webhookSecret"]; found {
		t.Errorf("expected missing secrets to be omitted: %v", values)
	}

	if err := repository.Delete(workspaceRoot, "development", "apiToken"); err != nil {
		t.Fatalf("delete secret: %v", err)
	}
	states, err = repository.States(workspaceRoot, "development")
	if err != nil {
		t.Fatalf("secret states after delete: %v", err)
	}
	if states[0].IsSet {
		t.Errorf("expected the secret to be deleted: %+v", states[0])
	}
}

func TestServiceRejectsUndeclaredSecret(t *testing.T) {
	workspaceRoot := createTestWorkspace(t)
	repository := NewRepository(newFakeKeychain())

	err := repository.Set(workspaceRoot, "development", "unknown", "value")
	if !errors.Is(err, ErrSecretNotDeclared) {
		t.Fatalf("expected ErrSecretNotDeclared, got %v", err)
	}
}

func TestKeychainServiceUsesWorkspaceIdentifier(t *testing.T) {
	workspaceRoot := createTestWorkspace(t)
	configuration, err := workspace.NewRepository(workspaceRoot).Load()
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}

	keychain := newFakeKeychain()
	repository := NewRepository(keychain)
	if err := repository.Set(workspaceRoot, "development", "apiToken", "value"); err != nil {
		t.Fatalf("set secret: %v", err)
	}

	key := "awake/" + configuration.ID + "\x00development/apiToken"
	if keychain.values[key] != "value" {
		t.Errorf("unexpected keychain content: %v", keychain.values)
	}
}

func TestKeychainSeparatesEnvironments(t *testing.T) {
	workspaceRoot := createTestWorkspace(t)
	path := filepath.Join(workspaceRoot, "environments", "production.yaml")
	if err := os.WriteFile(path, []byte("name: Production\nsecretVariables:\n  - apiToken\n"), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	repository := NewRepository(newFakeKeychain())
	if err := repository.Set(workspaceRoot, "development", "apiToken", "development-value"); err != nil {
		t.Fatalf("set development secret: %v", err)
	}
	if err := repository.Set(workspaceRoot, "production", "apiToken", "production-value"); err != nil {
		t.Fatalf("set production secret: %v", err)
	}

	development, err := repository.Values(workspaceRoot, "development")
	if err != nil {
		t.Fatalf("development values: %v", err)
	}
	production, err := repository.Values(workspaceRoot, "production")
	if err != nil {
		t.Fatalf("production values: %v", err)
	}

	if development["apiToken"] != "development-value" {
		t.Errorf("unexpected development value: %v", development)
	}
	if production["apiToken"] != "production-value" {
		t.Errorf("unexpected production value: %v", production)
	}
}

func createTestWorkspace(t *testing.T) string {
	t.Helper()

	workspaceRoot := t.TempDir()
	if _, err := workspace.Create(workspaceRoot, workspace.CreateOptions{
		Name: "Test",
		Mode: workspace.ModePersonal,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	environment := "name: Development\nvariables:\n  baseUrl: https://example.com\nsecretVariables:\n  - apiToken\n  - webhookSecret\n"
	path := filepath.Join(workspaceRoot, "environments", "development.yaml")
	if err := os.WriteFile(path, []byte(environment), 0o644); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	return workspaceRoot
}
