package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/clementherve/awake/src/features/workspace"
	"github.com/clementherve/awake/src/shared/variables"
)

func TestLoadVariableLayersInheritsFromGlobalEnvironment(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Layers", Mode: workspace.ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	write := func(path string, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("environments/staging.yaml", "name: Staging\nvariables:\n  baseUrl: https://global.example.com\n  region: eu\n")
	write("collections/billing/collection.yaml", "name: Billing\nvariables:\n  version: v2\n  region: collection-default\n")
	write("collections/billing/environments/staging.yaml", "name: Staging\nvariables:\n  baseUrl: https://billing.example.com\n")
	write("collections/billing/environments/staging.local.yaml", "variables:\n  token: mine\n")
	write("collections/other/collection.yaml", "name: Other\n")

	source := requestSource{}
	merged := func(requestPath string) map[string]string {
		t.Helper()
		layers, err := source.LoadVariableLayers(root, requestPath, "staging")
		if err != nil {
			t.Fatalf("load layers for %s: %v", requestPath, err)
		}
		result := map[string]string{}
		for _, values := range variables.Maps(layers) {
			for name, value := range values {
				result[name] = value
			}
		}
		return result
	}

	billing := merged("collections/billing/invoices/list.yaml")
	expected := map[string]string{
		"baseUrl": "https://billing.example.com",
		"region":  "eu",
		"version": "v2",
		"token":   "mine",
	}
	for name, value := range expected {
		if billing[name] != value {
			t.Errorf("billing %s: expected %q, got %q", name, value, billing[name])
		}
	}

	other := merged("collections/other/ping.yaml")
	if other["baseUrl"] != "https://global.example.com" || other["version"] != "" {
		t.Errorf("a collection without its own environment must inherit the global one, got %v", other)
	}

	if _, err := source.LoadVariableLayers(root, "collections/billing/list.yaml", "production"); err == nil {
		t.Error("expected an unknown environment to be reported")
	}
}

func TestLoadRequestInheritsTheCollectionAuthentication(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Auth", Mode: workspace.ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	write := func(path string, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("collections/billing/collection.yaml", "name: Billing\nauthentication:\n  type: bearer\n  token: \"{{token}}\"\n")
	write("collections/billing/invoices/list.yaml", "name: List\nmethod: GET\nurl: https://example.com\nauthentication:\n  type: inherit\n")
	write("collections/billing/public.yaml", "name: Public\nmethod: GET\nurl: https://example.com\nauthentication:\n  type: none\n")
	write("collections/open/collection.yaml", "name: Open\n")
	write("collections/open/ping.yaml", "name: Ping\nmethod: GET\nurl: https://example.com\n")

	source := requestSource{}
	inherited, err := source.LoadRequest(root, "collections/billing/invoices/list.yaml")
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	if inherited.Authentication == nil || inherited.Authentication.Type != "bearer" || inherited.Authentication.Token != "{{token}}" {
		t.Fatalf("expected the collection bearer token, got %+v", inherited.Authentication)
	}

	own, err := source.LoadRequest(root, "collections/billing/public.yaml")
	if err != nil || own.Authentication.Type != "none" {
		t.Fatalf("a request with its own authentication keeps it, got %+v, %v", own.Authentication, err)
	}

	open, err := source.LoadRequest(root, "collections/open/ping.yaml")
	if err != nil || open.Authentication.Type != "none" {
		t.Fatalf("without collection authentication the request sends none, got %+v, %v", open.Authentication, err)
	}
}
