package interop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestImportWithAPluginFormatWritesItsCollections(t *testing.T) {
	plugins := &fakePluginFormats{imported: `[{
		"name": "Insomnia API",
		"variables": {"baseUrl": "https://api.example.com"},
		"requests": [
			{"name": "List users", "request": {"method": "get", "url": "{{baseUrl}}/users", "headers": [{"name": "Accept", "value": "application/json", "enabled": true}]}},
			{"name": "Broken", "request": {"method": "GET"}}
		],
		"folders": [{"name": "Admin", "requests": [{"name": "Stats", "request": {"method": "GET", "url": "{{baseUrl}}/stats"}}]}],
		"environments": [{"name": "Production", "variables": {"baseUrl": "https://api.example.com"}, "secretVariables": ["token"]}]
	}]`}
	writer := newFakeWriter()
	service := NewService(writer, fakeSource{})
	SetPluginFormats(service, plugins)
	sourcePath := filepath.Join(t.TempDir(), "export.insomnia.json")
	if err := os.WriteFile(sourcePath, []byte("document"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := service.Import(t.TempDir(), sourcePath, "plugin:insomnia/insomnia")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if plugins.fileName != "export.insomnia.json" || plugins.content != "document" {
		t.Errorf("expected the plugin to read the document, got %q %q", plugins.fileName, plugins.content)
	}
	if result.Collections != 1 || result.Requests != 2 || result.Environments != 1 {
		t.Errorf("unexpected result: %+v", result)
	}
	if !slices.ContainsFunc(result.Warnings, func(warning string) bool { return strings.Contains(warning, "Insomnia API/Broken: skipped") }) {
		t.Errorf("expected the invalid request to be skipped with a warning, got %v", result.Warnings)
	}
	request, found := writer.requests["collections/insomnia-api/list-users.yaml"]
	if !found || request.Method != "GET" || len(request.Headers) != 1 {
		t.Errorf("unexpected requests: %+v", writer.requests)
	}
	if _, found := writer.requests["collections/insomnia-api/admin/stats.yaml"]; !found {
		t.Errorf("expected the folder request, got %v", writer.requests)
	}
}

func TestImportWithAPluginFormatRefusesUnknownFields(t *testing.T) {
	service := NewService(newFakeWriter(), fakeSource{})
	SetPluginFormats(service, &fakePluginFormats{imported: `[{"name": "API", "reqests": []}]`})
	sourcePath := filepath.Join(t.TempDir(), "document.json")
	if err := os.WriteFile(sourcePath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := service.Import(t.TempDir(), sourcePath, "plugin:insomnia/insomnia")

	if err == nil || !strings.Contains(err.Error(), "reqests") {
		t.Fatalf("expected the misspelt field to be reported, got %v", err)
	}
}

func TestExportWithAPluginFormatWritesItsDocument(t *testing.T) {
	plugins := &fakePluginFormats{exported: "har document"}
	service := NewService(newFakeWriter(), exportTestSource())
	SetPluginFormats(service, plugins)
	destination := filepath.Join(t.TempDir(), "workspace")

	result, err := service.Export(t.TempDir(), destination, "plugin:har/har", ExportOptions{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if len(result.Paths) != 1 || filepath.Ext(result.Paths[0]) != ".har" {
		t.Fatalf("expected one .har file, got %v", result.Paths)
	}
	content, err := os.ReadFile(result.Paths[0])
	if err != nil || string(content) != "har document" {
		t.Errorf("unexpected document %q: %v", content, err)
	}

	var collections []map[string]any
	if err := json.Unmarshal(plugins.collections, &collections); err != nil {
		t.Fatalf("decode what the plugin received: %v", err)
	}
	firstRequest := collections[0]["requests"].([]any)[0].(map[string]any)["request"].(map[string]any)
	if collections[0]["name"] != "Platform" || firstRequest["url"] != "{{baseUrl}}/status" || firstRequest["method"] != "GET" {
		t.Errorf("expected the workspace field names, got %s", plugins.collections)
	}
}

func TestPluginFormatsNeedAPluginHost(t *testing.T) {
	service := NewService(newFakeWriter(), exportTestSource())

	if _, err := service.Export(t.TempDir(), filepath.Join(t.TempDir(), "out"), "plugin:har/har", ExportOptions{}); !errors.Is(err, ErrPluginFormatsUnavailable) {
		t.Errorf("expected ErrPluginFormatsUnavailable, got %v", err)
	}
	if list := service.PluginFormats(t.TempDir()); list.Imports == nil || list.Exports == nil {
		t.Errorf("expected empty lists, got %+v", list)
	}

	SetPluginFormats(service, &fakePluginFormats{})
	if _, err := service.Export(t.TempDir(), filepath.Join(t.TempDir(), "out"), "plugin:other/format", ExportOptions{}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat for a format no plugin exports, got %v", err)
	}
}

type fakePluginFormats struct {
	imported    string
	exported    string
	fileName    string
	content     string
	collections []byte
}

func (plugins *fakePluginFormats) Formats(string) ([]PluginFormat, []PluginFormat) {
	return []PluginFormat{{Format: "plugin:insomnia/insomnia", Label: "Insomnia", Extensions: []string{".json"}}},
		[]PluginFormat{{Format: "plugin:har/har", Label: "HAR", Extensions: []string{".har"}}}
}

func (plugins *fakePluginFormats) Import(_ string, _ Format, fileName string, content []byte) ([]byte, error) {
	plugins.fileName, plugins.content = fileName, string(content)
	return []byte(plugins.imported), nil
}

func (plugins *fakePluginFormats) Export(_ string, _ Format, collections []byte) (string, error) {
	plugins.collections = collections
	return plugins.exported, nil
}
