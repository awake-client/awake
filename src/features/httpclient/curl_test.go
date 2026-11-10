package httpclient

import (
	"testing"

	"github.com/awake-client/awake/src/domain"
)

func TestCopyAsCurlResolvesHeadersAndBody(t *testing.T) {
	service := newServiceWithStore(fakeSource{
		request: domain.Request{
			Method: "POST",
			URL:    "{{baseUrl}}/users",
			Headers: []domain.Header{
				{Name: "X-Trace", Value: "{{trace}}", Enabled: true},
				{Name: "X-Disabled", Value: "no", Enabled: false},
			},
			Body:           &domain.Body{Type: domain.BodyJSON, Content: `{"name":"O'Brien"}`},
			Authentication: &domain.Authentication{Type: domain.AuthenticationBearer, Token: "{{token}}"},
		},
		layers: []map[string]string{{"baseUrl": "https://api.example.com", "trace": "abc", "token": "secret"}},
	}, nil, nil)

	command, err := service.CopyAsCurl("", "users.yaml", "")
	if err != nil {
		t.Fatalf("CopyAsCurl returned an error: %v", err)
	}

	expected := "curl -X POST 'https://api.example.com/users' \\\n" +
		"  -H 'Authorization: Bearer secret' \\\n" +
		"  -H 'Content-Type: application/json' \\\n" +
		"  -H 'User-Agent: Awake' \\\n" +
		"  -H 'X-Trace: abc' \\\n" +
		"  --data-raw '{\"name\":\"O'\\''Brien\"}'"
	if command != expected {
		t.Fatalf("unexpected command:\n%s\nwant:\n%s", command, expected)
	}
}

func TestCopyAsCurlOmitsMethodForPlainGet(t *testing.T) {
	service := newServiceWithStore(fakeSource{
		request: domain.Request{Method: "GET", URL: "https://api.example.com/health"},
	}, nil, nil)

	command, err := service.CopyAsCurl("", "health.yaml", "")
	if err != nil {
		t.Fatalf("CopyAsCurl returned an error: %v", err)
	}

	expected := "curl 'https://api.example.com/health' \\\n  -H 'User-Agent: Awake'"
	if command != expected {
		t.Fatalf("unexpected command:\n%s\nwant:\n%s", command, expected)
	}
}
