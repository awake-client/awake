package domain

import "testing"

func TestApplyDefaultsSetsHTTPProtocol(t *testing.T) {
	request := Request{Name: "List users", Method: "get", URL: "https://example.com/users"}
	request.ApplyDefaults()

	if request.Protocol != ProtocolHTTP {
		t.Fatalf("expected the HTTP protocol, got %q", request.Protocol)
	}
	if request.Method != MethodGet {
		t.Errorf("expected the method to be normalized, got %q", request.Method)
	}
	if request.Authentication == nil || request.Body == nil {
		t.Errorf("expected defaults for authentication and body")
	}
}

func TestValidateAcceptsStreamingProtocols(t *testing.T) {
	for _, protocol := range []Protocol{ProtocolHTTP, ProtocolSSE, ProtocolWebSocket} {
		request := Request{
			Name:     "Stream",
			Method:   MethodGet,
			URL:      "wss://example.com/socket",
			Protocol: protocol,
		}
		if err := request.Validate(); err != nil {
			t.Errorf("expected protocol %q to be valid, got %v", protocol, err)
		}
	}
}

func TestValidateRejectsUnknownProtocol(t *testing.T) {
	request := Request{
		Name:     "Stream",
		Method:   MethodGet,
		URL:      "https://example.com",
		Protocol: Protocol("grpc"),
	}

	if err := request.Validate(); err != ErrInvalidRequest {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}
