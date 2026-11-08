package interop

import (
	"testing"

	"github.com/awake-client/awake/src/domain"
)

func TestReadPostmanConvertsFoldersAndAuth(t *testing.T) {
	document := []byte(`{
	  "info": {"name": "Platform"},
	  "variable": [{"key": "baseUrl", "value": "https://api.example.com"}],
	  "item": [
	    {
	      "name": "Users",
	      "item": [
	        {
	          "name": "List users",
	          "request": {
	            "method": "GET",
	            "header": [{"key": "Accept", "value": "application/json"}],
	            "url": {
	              "raw": "{{baseUrl}}/users",
	              "query": [{"key": "page", "value": "1"}]
	            }
	          }
	        },
	        {
	          "name": "Create user",
	          "request": {
	            "method": "POST",
	            "url": "{{baseUrl}}/users",
	            "body": {
	              "mode": "raw",
	              "raw": "{\"name\":\"Ada\"}",
	              "options": {"raw": {"language": "json"}}
	            },
	            "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "{{token}}"}]}
	          }
	        }
	      ]
	    }
	  ]
	}`)

	collections, err := ReadPostman(document)
	if err != nil {
		t.Fatalf("read postman: %v", err)
	}

	collection := collections[0]
	if collection.Name != "Platform" || collection.Variables["baseUrl"] != "https://api.example.com" {
		t.Errorf("unexpected collection: %+v", collection)
	}
	if len(collection.Folders) != 1 || len(collection.Folders[0].Requests) != 2 {
		t.Fatalf("unexpected folders: %+v", collection.Folders)
	}

	listRequest := collection.Folders[0].Requests[0]
	if listRequest.Request.URL != "{{baseUrl}}/users" {
		t.Errorf("unexpected url %q", listRequest.Request.URL)
	}
	if len(listRequest.Request.Parameters) != 1 || listRequest.Request.Parameters[0].Name != "page" {
		t.Errorf("unexpected parameters: %+v", listRequest.Request.Parameters)
	}

	createRequest := collection.Folders[0].Requests[1]
	if createRequest.Request.Body == nil || createRequest.Request.Body.Type != domain.BodyJSON {
		t.Fatalf("unexpected body: %+v", createRequest.Request.Body)
	}
	if createRequest.Request.Authentication == nil ||
		createRequest.Request.Authentication.Type != domain.AuthenticationBearer ||
		createRequest.Request.Authentication.Token != "{{token}}" {
		t.Errorf("unexpected authentication: %+v", createRequest.Request.Authentication)
	}
}

func TestPostmanRoundTrip(t *testing.T) {
	collection := Collection{
		Name:          "Platform",
		Documentation: "Users.",
		Variables:     map[string]string{"baseUrl": "https://api.example.com"},
		Requests: []Request{{
			Name: "List users",
			Request: domain.Request{
				Name:   "List users",
				Method: domain.MethodGet,
				URL:    "{{baseUrl}}/users",
				Headers: []domain.Header{
					{Name: "Accept", Value: "application/json", Enabled: true},
				},
				Parameters: []domain.Parameter{
					{Name: "page", Value: "1", Enabled: true},
				},
				Authentication: &domain.Authentication{
					Type:  domain.AuthenticationBearer,
					Token: "{{token}}",
				},
			},
		}},
	}

	content, err := WritePostman([]Collection{collection})
	if err != nil {
		t.Fatalf("write postman: %v", err)
	}

	decoded, err := ReadPostman(content)
	if err != nil {
		t.Fatalf("read written postman: %v", err)
	}

	if decoded[0].Variables["baseUrl"] != "https://api.example.com" {
		t.Errorf("unexpected variables: %+v", decoded[0].Variables)
	}
	if len(decoded[0].Folders) != 1 || len(decoded[0].Folders[0].Requests) != 1 {
		t.Fatalf("unexpected items: %+v", decoded[0])
	}

	request := decoded[0].Folders[0].Requests[0].Request
	if request.URL != "{{baseUrl}}/users" {
		t.Errorf("unexpected url %q", request.URL)
	}
	if len(request.Parameters) != 1 || request.Parameters[0].Value != "1" {
		t.Errorf("unexpected parameters: %+v", request.Parameters)
	}
	if request.Authentication == nil || request.Authentication.Token != "{{token}}" {
		t.Errorf("unexpected authentication: %+v", request.Authentication)
	}
}
