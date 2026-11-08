package app

import (
	"path/filepath"
	"testing"
)

func TestLoadCollectionEnvironmentsMergesLocalOverridesAndHidesSecrets(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeFile(t, filepath.Join(workspaceRoot, "collections", "platform", "environments", "production.yaml"), `
name: Production
variables:
  baseUrl: https://api.example.com
  region: eu
secretVariables:
  - token
`)
	writeFile(t, filepath.Join(workspaceRoot, "collections", "platform", "environments", "production.local.yaml"), `
variables:
  baseUrl: https://staging.example.com
  extra: local
`)

	loaded, err := loadCollectionEnvironments(workspaceRoot, "collections/platform")
	if err != nil {
		t.Fatalf("load environments: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected one environment, got %+v", loaded)
	}

	environment := loaded[0]
	if environment.Name != "Production" {
		t.Errorf("unexpected name %q", environment.Name)
	}
	if environment.Variables["baseUrl"] != "https://staging.example.com" ||
		environment.Variables["region"] != "eu" ||
		environment.Variables["extra"] != "local" {
		t.Errorf("unexpected variables: %+v", environment.Variables)
	}
	if _, exposed := environment.Variables["token"]; exposed {
		t.Error("a secret variable kept its value")
	}
	if len(environment.SecretVariables) != 1 || environment.SecretVariables[0] != "token" {
		t.Errorf("unexpected secret variables: %+v", environment.SecretVariables)
	}
}
