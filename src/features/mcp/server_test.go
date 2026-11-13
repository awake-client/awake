package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeWorkspace struct {
	Workspace
	created map[string]domain.Request
}

func (fake *fakeWorkspace) CreateRequest(path string, request domain.Request) error {
	if fake.created == nil {
		fake.created = map[string]domain.Request{}
	}
	fake.created[path] = request
	return nil
}

func (fake *fakeWorkspace) LoadRequest(path string) (domain.Request, error) {
	request, found := fake.created[path]
	if !found {
		return domain.Request{}, ErrNotFound
	}
	return request, nil
}

func (*fakeWorkspace) DefaultEnvironment() string { return "staging" }
func (*fakeWorkspace) Guide() string              { return "# Guide" }

type fakeSender struct {
	Sender
	environment string
}

func (fake *fakeSender) Variables(_ string, environment string) ([]Variable, error) {
	fake.environment = environment
	return []Variable{
		{Name: "baseUrl", Value: "https://example.com", Source: "environment"},
		{Name: "token", Value: "", Source: "secret", Secret: true},
		{Name: "password", Value: "typed-in-a-private-environment", Source: "environment (private)"},
	}, nil
}

func connect(t *testing.T, workspace Workspace, sender Sender, options Options) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := NewServer(workspace, sender, nil, options)
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func call(t *testing.T, session *sdk.ClientSession, name string, arguments map[string]any) (*sdk.CallToolResult, map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	output := map[string]any{}
	if result.StructuredContent != nil {
		content, _ := json.Marshal(result.StructuredContent)
		_ = json.Unmarshal(content, &output)
	}
	return result, output
}

func toolNames(t *testing.T, session *sdk.ClientSession) []string {
	t.Helper()
	var names []string
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	return names
}

func TestReadOnlyServersLeaveOutTheWriteTools(t *testing.T) {
	all := strings.Join(toolNames(t, connect(t, &fakeWorkspace{}, &fakeSender{}, Options{})), ",")
	for _, name := range []string{"list_requests", "get_request", "send_request", "create_request", "update_request", "create_folder", "create_collection"} {
		if !strings.Contains(all, name) {
			t.Errorf("expected the tool %s, got %s", name, all)
		}
	}

	readOnly := strings.Join(toolNames(t, connect(t, &fakeWorkspace{}, &fakeSender{}, Options{ReadOnly: true})), ",")
	if strings.Contains(readOnly, "create_") || strings.Contains(readOnly, "update_") {
		t.Errorf("a read-only server must not offer write tools, got %s", readOnly)
	}
	if !strings.Contains(readOnly, "send_request") {
		t.Errorf("a read-only server still sends requests, got %s", readOnly)
	}
}

func TestGetVariablesMasksSecretsAndUsesTheDefaultEnvironment(t *testing.T) {
	sender := &fakeSender{}
	session := connect(t, &fakeWorkspace{}, sender, Options{})

	_, output := call(t, session, "get_variables", map[string]any{"requestPath": "collections/a/b.yaml"})
	if sender.environment != "staging" || output["environment"] != "staging" {
		t.Errorf("expected the default environment, got %q", sender.environment)
	}
	variables, _ := output["variables"].([]any)
	token, _ := variables[1].(map[string]any)
	if token["value"] != maskedValue {
		t.Errorf("expected the secret to be masked, got %v", token)
	}
	password, _ := variables[2].(map[string]any)
	if password["value"] != maskedValue {
		t.Errorf("expected a credential stored as a plain variable to be masked, got %v", password)
	}
	baseURL, _ := variables[0].(map[string]any)
	if baseURL["value"] != "https://example.com" {
		t.Errorf("ordinary variables must stay readable, got %v", baseURL)
	}

	call(t, session, "get_variables", map[string]any{"requestPath": "collections/a/b.yaml", "environment": ""})
	if sender.environment != "" {
		t.Errorf("an empty environment must select none, got %q", sender.environment)
	}
}

func TestCreateRequestUsesTheFileFieldsAndRejectsUnknownOnes(t *testing.T) {
	workspace := &fakeWorkspace{}
	session := connect(t, workspace, &fakeSender{}, Options{})

	_, output := call(t, session, "create_request", map[string]any{
		"path": "collections/a/new.yaml",
		"request": map[string]any{
			"name":    "New",
			"method":  "POST",
			"url":     "{{baseUrl}}/items",
			"headers": []any{map[string]any{"name": "Accept", "value": "application/json", "enabled": true}},
		},
	})
	created := workspace.created["collections/a/new.yaml"]
	if created.Method != "POST" || created.URL != "{{baseUrl}}/items" || len(created.Headers) != 1 {
		t.Fatalf("unexpected request: %+v", created)
	}
	request, _ := output["request"].(map[string]any)
	if request["url"] != "{{baseUrl}}/items" {
		t.Errorf("expected the saved request in the file format, got %v", output)
	}

	result, _ := call(t, session, "create_request", map[string]any{
		"path":    "collections/a/typo.yaml",
		"request": map[string]any{"name": "Typo", "metod": "GET"},
	})
	if !result.IsError {
		t.Error("expected an unknown field to be rejected")
	}
}

func TestTheGuideIsAResource(t *testing.T) {
	session := connect(t, &fakeWorkspace{}, &fakeSender{}, Options{})
	result, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: GuideURI})
	if err != nil || len(result.Contents) != 1 || result.Contents[0].Text != "# Guide" {
		t.Fatalf("unexpected guide: %+v, %v", result, err)
	}
}

func TestTruncateTextKeepsWholeCharacters(t *testing.T) {
	text, cut := truncateText("héllo", 2)
	if text != "h" || !cut {
		t.Errorf("expected the accented letter to be left out whole, got %q", text)
	}
	if text, cut := truncateText("short", 10); text != "short" || cut {
		t.Errorf("unexpected truncation: %q", text)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Billing API":  "billing-api",
		"  Users  v2 ": "users-v2",
		"Équipe!":      "equipe",
		"!!!":          "",
	}
	for name, expected := range cases {
		if actual := slugify(name); actual != expected {
			t.Errorf("%q: expected %q, got %q", name, expected, actual)
		}
	}
}

func TestSettingsTurnTheServerOffAndOn(t *testing.T) {
	root := t.TempDir()
	if enabled, err := Enabled(root); err != nil || !enabled {
		t.Fatalf("a workspace without settings must be enabled, got %v, %v", enabled, err)
	}
	if err := SetEnabled(root, false); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := Enabled(root); enabled {
		t.Error("expected the server to be disabled")
	}
	if err := SetEnabled(root, true); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := Enabled(root); !enabled {
		t.Error("expected the server to be enabled again")
	}
}

func TestADisabledServerRefusesCallsUntilTurnedBackOn(t *testing.T) {
	enabled := false
	session := connect(t, &fakeWorkspace{}, &fakeSender{}, Options{Enabled: func() bool { return enabled }})

	result, _ := call(t, session, "get_variables", map[string]any{"requestPath": "collections/a/b.yaml"})
	if !result.IsError {
		t.Fatal("expected a disabled server to refuse the call")
	}
	text, _ := result.Content[0].(*sdk.TextContent)
	if text == nil || !strings.Contains(text.Text, "disabled") {
		t.Errorf("expected the reason in the error, got %+v", result.Content)
	}
	if _, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: GuideURI}); err == nil {
		t.Error("expected a disabled server to refuse reading the guide")
	}

	enabled = true
	if result, _ := call(t, session, "get_variables", map[string]any{"requestPath": "collections/a/b.yaml"}); result.IsError {
		t.Errorf("expected the call to work once enabled, got %+v", result.Content)
	}
}
