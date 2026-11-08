package interop

import (
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
)

func TestReadOpenAPIConvertsOperationsToRequests(t *testing.T) {
	document := []byte(`
openapi: 3.1.0
info:
  title: Platform API
  description: Users and accounts.
servers:
  - url: https://api.example.com
paths:
  /users/{id}:
    parameters:
      - name: id
        in: path
        required: true
    get:
      summary: Get user
      tags:
        - Users
      parameters:
        - name: expand
          in: query
          schema:
            default: profile
    post:
      summary: Update user
      tags:
        - Users
      requestBody:
        content:
          application/json:
            example:
              name: Ada
`)

	collections, err := ReadOpenAPI(document)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}

	if len(collections) != 1 {
		t.Fatalf("expected one collection, got %d", len(collections))
	}

	collection := collections[0]
	if collection.Name != "Platform API" || collection.Documentation != "Users and accounts." {
		t.Errorf("unexpected collection: %+v", collection)
	}
	if collection.Variables["baseUrl"] != "https://api.example.com" {
		t.Errorf("unexpected variables: %+v", collection.Variables)
	}
	if len(collection.Folders) != 1 || collection.Folders[0].Name != "Users" {
		t.Fatalf("expected a Users folder, got %+v", collection.Folders)
	}

	requests := collection.Folders[0].Requests
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}

	getRequest := requests[0]
	if getRequest.Name != "Get user" || getRequest.Request.Method != domain.MethodGet {
		t.Errorf("unexpected get request: %+v", getRequest)
	}
	if getRequest.Request.URL != "{{baseUrl}}/users/:id" {
		t.Errorf("unexpected url %q", getRequest.Request.URL)
	}
	if len(getRequest.Request.PathParameters) != 1 || getRequest.Request.PathParameters[0].Name != "id" {
		t.Errorf("expected the id path parameter, got %+v", getRequest.Request.PathParameters)
	}
	if len(getRequest.Request.Parameters) != 1 || getRequest.Request.Parameters[0].Value != "profile" {
		t.Errorf("unexpected query parameters: %+v", getRequest.Request.Parameters)
	}

	postRequest := requests[1]
	if postRequest.Request.Body == nil || postRequest.Request.Body.Type != domain.BodyJSON {
		t.Fatalf("unexpected body: %+v", postRequest.Request.Body)
	}
	if strings.TrimSpace(postRequest.Request.Body.Content) != `{"name":"Ada"}` {
		t.Errorf("unexpected body content %q", postRequest.Request.Body.Content)
	}
}

func TestReadOpenAPIGroupsUntaggedRequestsAtCollectionRoot(t *testing.T) {
	document := []byte(`
openapi: 3.0.0
info:
  title: Billing
paths:
  /invoices:
    get:
      operationId: listInvoices
`)

	collections, err := ReadOpenAPI(document)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}

	if len(collections[0].Requests) != 1 {
		t.Fatalf("expected one request, got %+v", collections[0])
	}
	if collections[0].Requests[0].Name != "listInvoices" {
		t.Errorf("unexpected request name %q", collections[0].Requests[0].Name)
	}
}

func TestWriteOpenAPIProducesReadableDocument(t *testing.T) {
	collection := Collection{
		Name: "Platform",
		Requests: []Request{{
			Name: "List users",
			Request: domain.Request{
				Name:   "List users",
				Method: domain.MethodGet,
				URL:    "{{baseUrl}}/users",
				Parameters: []domain.Parameter{
					{Name: "page", Value: "1", Enabled: true},
				},
			},
		}},
	}

	content, err := WriteOpenAPI([]Collection{collection})
	if err != nil {
		t.Fatalf("write openapi: %v", err)
	}

	decoded, err := ReadOpenAPI(content)
	if err != nil {
		t.Fatalf("read written openapi: %v", err)
	}
	if len(decoded[0].Folders) != 1 || len(decoded[0].Folders[0].Requests) != 1 {
		t.Fatalf("expected one tagged request, got %+v", decoded[0])
	}
	if decoded[0].Folders[0].Requests[0].Request.URL != "{{baseUrl}}/users" {
		t.Errorf("unexpected url %q", decoded[0].Folders[0].Requests[0].Request.URL)
	}
}

func TestDetectDocumentFormat(t *testing.T) {
	testCases := []struct {
		path     string
		content  string
		expected Format
	}{
		{path: "openapi.yaml", content: "openapi: 3.1.0", expected: FormatOpenAPI},
		{path: "collection.json", content: `{"info":{},"item":[]}`, expected: FormatPostman},
		{path: "request.bru", content: "meta {", expected: FormatBruno},
		{path: "document.json", content: `{"openapi":"3.1.0"}`, expected: FormatOpenAPI},
	}

	for _, testCase := range testCases {
		detected := detectDocumentFormat(testCase.path, []byte(testCase.content))
		if detected != testCase.expected {
			t.Errorf("detect %s: expected %q, got %q", testCase.path, testCase.expected, detected)
		}
	}
}
