package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/workspace"
	"github.com/awake-client/awake/src/shared/variables"
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

func TestLoadRequestInheritsAuthenticationWithoutWritingIt(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "AutomaticAuth", Mode: workspace.ModeTeam}); err != nil {
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
	write("collections/billing/collection.yaml", "name: Billing\nauthentication:\n  type: bearer\n  token: collection-token\n")
	write("collections/billing/users/folder.yaml", "name: Users\nauthentication:\n  type: basic\n  username: folder-user\n")
	write("collections/billing/users/invoices/list.yaml", "name: List\nmethod: GET\nurl: https://example.com\n")
	write("collections/billing/users/admins/folder.yaml", "name: Admins\nauthentication:\n  type: apiKey\n  key: X-Api-Key\n  value: admin-key\n")
	write("collections/billing/users/admins/report.yaml", "name: Report\nmethod: GET\nurl: https://example.com\n")

	source := requestSource{}
	inherited, err := source.LoadRequest(root, "collections/billing/users/invoices/list.yaml")
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	if inherited.Authentication == nil || inherited.Authentication.Type != domain.AuthenticationBasic || inherited.Authentication.Username != "folder-user" {
		t.Fatalf("expected the closest folder authentication, got %+v", inherited.Authentication)
	}

	nested, err := source.LoadRequest(root, "collections/billing/users/admins/report.yaml")
	if err != nil {
		t.Fatalf("load nested request: %v", err)
	}
	if nested.Authentication == nil || nested.Authentication.Type != domain.AuthenticationAPIKey || nested.Authentication.Value != "admin-key" {
		t.Fatalf("expected the nested folder authentication, got %+v", nested.Authentication)
	}
}

func TestLoadRequestAddsTheCollectionAndFolderHeaders(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Headers", Mode: workspace.ModeTeam}); err != nil {
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
	write("collections/billing/collection.yaml", "name: Billing\nheaders:\n  - {name: Accept, value: application/json, enabled: true}\n  - {name: X-Scope, value: collection, enabled: true}\n")
	write("collections/billing/users/folder.yaml", "name: Users\nheaders:\n  - {name: X-Scope, value: folder, enabled: true}\n  - {name: X-Folder, value: users, enabled: true}\n")
	write("collections/billing/users/list.yaml", "name: List\nmethod: GET\nurl: https://example.com\nheaders:\n  - {name: X-Scope, value: request, enabled: true}\n  - {name: X-Own, value: request, enabled: true}\n")

	loaded, err := requestSource{}.LoadRequest(root, "collections/billing/users/list.yaml")
	if err != nil {
		t.Fatalf("load request: %v", err)
	}

	names := make([]string, 0, len(loaded.Headers))
	for _, header := range loaded.Headers {
		names = append(names, header.Name+"="+header.Value)
	}
	expected := []string{
		"Accept=application/json",
		"X-Scope=collection",
		"X-Scope=folder",
		"X-Folder=users",
		"X-Scope=request",
		"X-Own=request",
	}
	if len(names) != len(expected) {
		t.Fatalf("expected headers %v, got %v", expected, names)
	}
	for index, name := range names {
		if name != expected[index] {
			t.Errorf("header %d: expected %q, got %q", index, expected[index], name)
		}
	}
}

func TestLoadRequestAppliesLocalOverrides(t *testing.T) {
	root := t.TempDir()
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
	write("collections/billing/invoices.yaml", "name: Invoices\nmethod: POST\nurl: https://example.com\nparameters:\n  - name: page\n    value: \"1\"\n    enabled: true\nbody:\n  type: json\n  content: '{\"shared\":true}'\n")
	write("collections/billing/invoices.local.yaml", "body:\n  type: json\n  content: '{\"local\":true}'\n")

	request, err := requestSource{}.LoadRequest(root, "collections/billing/invoices.yaml")
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	if request.Body == nil || request.Body.Content != `{"local":true}` {
		t.Errorf("expected the local body, got %+v", request.Body)
	}
	if len(request.Parameters) != 1 || request.Parameters[0].Value != "1" {
		t.Errorf("parameters are not overridden and must stay shared, got %+v", request.Parameters)
	}
}

func TestLoadVariableLayersFoldsIntoTheFolderOfARequest(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Folders", Mode: workspace.ModeTeam}); err != nil {
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
	write("collections/billing/users/folder.yaml", "name: Users\nvariables:\n  region: folder-default\n  pageSize: \"20\"\n")

	source := requestSource{}
	layers, err := source.LoadVariableLayers(root, "collections/billing/users/invoices/list.yaml", "staging")
	if err != nil {
		t.Fatalf("load layers: %v", err)
	}
	merged := map[string]string{}
	for _, values := range variables.Maps(layers) {
		for name, value := range values {
			merged[name] = value
		}
	}
	if merged["version"] != "v2" || merged["region"] != "eu" || merged["baseUrl"] != "https://global.example.com" || merged["pageSize"] != "20" {
		t.Errorf("unexpected merged variables: %v", merged)
	}
}

func TestLoadRequestInheritsTheFolderAuthentication(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "AuthFolders", Mode: workspace.ModeTeam}); err != nil {
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
	write("collections/billing/collection.yaml", "name: Billing\nauthentication:\n  type: bearer\n  token: collection-token\n")
	write("collections/billing/users/folder.yaml", "name: Users\nauthentication:\n  type: basic\n  username: folder-user\n")
	write("collections/billing/users/invoices/list.yaml", "name: List\nmethod: GET\nurl: https://example.com\nauthentication:\n  type: inherit\n")
	write("collections/billing/invoices.yaml", "name: Invoices\nmethod: GET\nurl: https://example.com\nauthentication:\n  type: inherit\n")

	source := requestSource{}
	inherited, err := source.LoadRequest(root, "collections/billing/users/invoices/list.yaml")
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	if inherited.Authentication == nil || inherited.Authentication.Type != domain.AuthenticationBasic || inherited.Authentication.Username != "folder-user" {
		t.Fatalf("expected the folder authentication, got %+v", inherited.Authentication)
	}

	fallback, err := source.LoadRequest(root, "collections/billing/invoices.yaml")
	if err != nil {
		t.Fatalf("load request: %v", err)
	}
	if fallback.Authentication == nil || fallback.Authentication.Type != domain.AuthenticationBearer || fallback.Authentication.Token != "collection-token" {
		t.Fatalf("expected the collection authentication fallback, got %+v", fallback.Authentication)
	}
}

func TestResolveAuthenticationReturnsTheDefiningEntry(t *testing.T) {
	root := t.TempDir()
	if _, err := workspace.NewService().Create(root, workspace.CreateOptions{Name: "Owners", Mode: workspace.ModeTeam}); err != nil {
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
	write("collections/keycloak/collection.yaml", "name: Keycloak\nauthentication:\n  type: oauth2\n  grantType: clientCredentials\n  tokenUrl: https://id.example.com/token\n  clientId: app\n")
	write("collections/keycloak/users/folder.yaml", "name: Users\nauthentication:\n  type: inherit\n")
	write("collections/keycloak/users/list.yaml", "name: List\nmethod: GET\nurl: https://example.com\n")
	write("collections/keycloak/own.yaml", "name: Own\nmethod: GET\nurl: https://example.com\nauthentication:\n  type: bearer\n  token: mine\n")
	write("collections/open/collection.yaml", "name: Open\n")

	source := requestSource{}
	cases := []struct {
		entry    string
		owner    string
		authType domain.AuthenticationType
	}{
		{"collections/keycloak", "collections/keycloak", domain.AuthenticationOAuth2},
		{"collections/keycloak/users", "collections/keycloak", domain.AuthenticationOAuth2},
		{"collections/keycloak/users/list.yaml", "collections/keycloak", domain.AuthenticationOAuth2},
		{"collections/keycloak/own.yaml", "collections/keycloak/own.yaml", domain.AuthenticationBearer},
		{"collections/open", "collections/open", domain.AuthenticationNone},
	}
	for _, c := range cases {
		authentication, owner, err := source.ResolveAuthentication(root, c.entry)
		if err != nil {
			t.Fatalf("resolve %s: %v", c.entry, err)
		}
		if owner != c.owner || authentication.Type != c.authType {
			t.Errorf("%s: expected %s from %s, got %s from %s", c.entry, c.authType, c.owner, authentication.Type, owner)
		}
	}

	layers, err := source.LoadVariableLayers(root, "collections/keycloak", "")
	if err != nil || len(layers) == 0 {
		t.Errorf("expected the variables of a collection, got %v, %v", layers, err)
	}
}
