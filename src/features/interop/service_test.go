package interop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/clementherve/awake/src/domain"
)

type fakeWriter struct {
	collections []string
	requests    map[string]domain.Request
}

func newFakeWriter() *fakeWriter {
	return &fakeWriter{requests: map[string]domain.Request{}}
}

func (writer *fakeWriter) CreateCollection(_ string, relativePath string, _ domain.Collection) error {
	writer.collections = append(writer.collections, relativePath)
	return nil
}

func (writer *fakeWriter) CreateRequest(_ string, relativePath string, request domain.Request) error {
	writer.requests[relativePath] = request
	return nil
}

type fakeSource struct {
	collections []CollectionSnapshot
}

func (source fakeSource) LoadCollections(_ string) ([]CollectionSnapshot, error) {
	return source.collections, nil
}

func TestImportWritesCollectionsAndRequests(t *testing.T) {
	document := []byte(`
openapi: 3.1.0
info:
  title: Platform API
paths:
  /users:
    get:
      summary: List users
  /users/{id}:
    delete:
      summary: Delete user
`)

	sourcePath := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(sourcePath, document, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	writer := newFakeWriter()
	service := NewService(writer, fakeSource{})

	result, err := service.Import(t.TempDir(), sourcePath, FormatOpenAPI)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	if result.Collections != 1 || result.Requests != 2 {
		t.Errorf("unexpected result: %+v", result)
	}
	if len(writer.collections) != 1 || writer.collections[0] != "collections/platform-api" {
		t.Errorf("unexpected collections: %v", writer.collections)
	}
	if len(writer.requests) != 2 {
		t.Fatalf("unexpected requests: %v", writer.requests)
	}
	if _, found := writer.requests["collections/platform-api/list-users.yaml"]; !found {
		t.Errorf("expected list-users request, got %v", writer.requests)
	}
}

func TestExportOpenAPIWritesAFile(t *testing.T) {
	service := NewService(newFakeWriter(), fakeSource{
		collections: []CollectionSnapshot{{
			Name: "Platform",
			Requests: []RequestSnapshot{{
				RelativePath: "users/list.yaml",
				Request: domain.Request{
					Name:   "List users",
					Method: domain.MethodGet,
					URL:    "{{baseUrl}}/users",
				},
			}},
		}},
	})

	destination := filepath.Join(t.TempDir(), "export")
	result, err := service.Export(t.TempDir(), destination, FormatOpenAPI)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	expectedPath := destination + ".yaml"
	if len(result.Paths) != 1 || result.Paths[0] != expectedPath {
		t.Fatalf("unexpected paths: %v", result.Paths)
	}

	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	collections, err := ReadOpenAPI(content)
	if err != nil {
		t.Fatalf("read exported openapi: %v", err)
	}
	if len(collections[0].Folders) != 1 || len(collections[0].Folders[0].Requests) != 1 {
		t.Errorf("unexpected exported tree: %+v", collections[0])
	}
}
