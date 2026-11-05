package interop

import (
	"strings"
	"testing"

	"github.com/clementherve/awake/src/domain"
)

func TestReadBrunoFileParsesBlocks(t *testing.T) {
	content := []byte(`
meta {
  name: List users
  type: http
}

get {
  url: {{baseUrl}}/users
  body: none
  auth: bearer
}

params:query {
  page: 1
  ~draft: true
}

headers {
  Accept: application/json
}

body:json {
  {
    "name": "Ada"
  }
}

auth:bearer {
  token: {{token}}
}

docs {
  Returns a paginated list of users.
}
`)

	request, err := ReadBrunoFile(content, "list-users")
	if err != nil {
		t.Fatalf("read bruno file: %v", err)
	}

	if request.Name != "List users" || request.URL != "{{baseUrl}}/users" {
		t.Errorf("unexpected request: %+v", request)
	}
	if request.Method != domain.MethodGet {
		t.Errorf("unexpected method %q", request.Method)
	}
	if len(request.Parameters) != 2 || request.Parameters[0].Value != "1" || request.Parameters[1].Enabled {
		t.Errorf("unexpected parameters: %+v", request.Parameters)
	}
	if len(request.Headers) != 1 || request.Headers[0].Name != "Accept" {
		t.Errorf("unexpected headers: %+v", request.Headers)
	}
	if request.Body == nil || request.Body.Type != domain.BodyJSON {
		t.Fatalf("unexpected body: %+v", request.Body)
	}
	if !strings.Contains(request.Body.Content, `"name": "Ada"`) {
		t.Errorf("unexpected body content %q", request.Body.Content)
	}
	if request.Authentication == nil || request.Authentication.Token != "{{token}}" {
		t.Errorf("unexpected authentication: %+v", request.Authentication)
	}
	if !strings.Contains(request.Documentation, "paginated") {
		t.Errorf("unexpected documentation %q", request.Documentation)
	}
}

func TestBrunoDirectoryRoundTrip(t *testing.T) {
	collection := Collection{
		Name:          "Platform",
		Documentation: "Platform requests.",
		Variables:     map[string]string{"baseUrl": "https://api.example.com"},
		Requests: []Request{{
			Name: "Status",
			Request: domain.Request{
				Name:   "Status",
				Method: domain.MethodGet,
				URL:    "{{baseUrl}}/status",
			},
		}},
		Folders: []Folder{{
			Name: "Users",
			Requests: []Request{{
				Name: "Create user",
				Request: domain.Request{
					Name:   "Create user",
					Method: domain.MethodPost,
					URL:    "{{baseUrl}}/users",
					Body:   &domain.Body{Type: domain.BodyJSON, Content: `{"name": "Ada"}`},
				},
			}},
		}},
	}

	destination := t.TempDir()
	written, err := WriteBrunoDirectory(destination, []Collection{collection})
	if err != nil {
		t.Fatalf("write bruno directory: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(written), written)
	}

	decoded, err := ReadBrunoDirectory(destination)
	if err != nil {
		t.Fatalf("read bruno directory: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected one collection, got %d", len(decoded))
	}
	if decoded[0].Name != "Platform" || decoded[0].Variables["baseUrl"] != "https://api.example.com" {
		t.Errorf("unexpected collection: %+v", decoded[0])
	}
	if len(decoded[0].Requests) != 1 || len(decoded[0].Folders) != 1 {
		t.Fatalf("unexpected tree: %+v", decoded[0])
	}
	if decoded[0].Folders[0].Requests[0].Request.Body.Type != domain.BodyJSON {
		t.Errorf("unexpected body: %+v", decoded[0].Folders[0].Requests[0].Request.Body)
	}
}
