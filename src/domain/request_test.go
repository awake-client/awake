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

func TestApplyDefaultsSelectsAuthorizationCodeFlow(t *testing.T) {
	request := Request{
		Name:   "List users",
		Method: MethodGet,
		URL:    "https://example.com",
		Authentication: &Authentication{
			Type: AuthenticationOAuth2,
		},
	}
	request.ApplyDefaults()

	if request.Authentication.GrantType != OAuth2GrantAuthorizationCode {
		t.Fatalf("expected the authorization code grant, got %q", request.Authentication.GrantType)
	}
}

func TestValidateAcceptsOAuth2Grants(t *testing.T) {
	tests := []struct {
		name           string
		authentication Authentication
	}{
		{
			name: "authorization code",
			authentication: Authentication{
				Type:             AuthenticationOAuth2,
				GrantType:        OAuth2GrantAuthorizationCode,
				AuthorizationURL: "https://example.com/authorize",
				TokenURL:         "https://example.com/token",
				ClientID:         "client-id",
			},
		},
		{
			name: "client credentials",
			authentication: Authentication{
				Type:      AuthenticationOAuth2,
				GrantType: OAuth2GrantClientCredentials,
				TokenURL:  "https://example.com/token",
				ClientID:  "client-id",
			},
		},
		{
			name: "static token",
			authentication: Authentication{
				Type:  AuthenticationOAuth2,
				Token: "access-token",
			},
		},
	}

	for _, test := range tests {
		request := Request{
			Name:           test.name,
			Method:         MethodGet,
			URL:            "https://example.com",
			Authentication: &test.authentication,
		}
		if err := request.Validate(); err != nil {
			t.Errorf("expected %s to be valid, got %v", test.name, err)
		}
	}
}

func TestValidateRejectsIncompleteOAuth2(t *testing.T) {
	tests := []struct {
		name           string
		authentication Authentication
	}{
		{
			name: "missing token URL",
			authentication: Authentication{
				Type:      AuthenticationOAuth2,
				GrantType: OAuth2GrantClientCredentials,
				ClientID:  "client-id",
			},
		},
		{
			name: "missing authorization URL",
			authentication: Authentication{
				Type:      AuthenticationOAuth2,
				GrantType: OAuth2GrantAuthorizationCode,
				TokenURL:  "https://example.com/token",
				ClientID:  "client-id",
			},
		},
		{
			name: "unknown grant",
			authentication: Authentication{
				Type:      AuthenticationOAuth2,
				GrantType: OAuth2GrantType("password"),
				TokenURL:  "https://example.com/token",
				ClientID:  "client-id",
			},
		},
	}

	for _, test := range tests {
		request := Request{
			Name:           test.name,
			Method:         MethodGet,
			URL:            "https://example.com",
			Authentication: &test.authentication,
		}
		if err := request.Validate(); err != ErrInvalidRequest {
			t.Errorf("expected %s to fail, got %v", test.name, err)
		}
	}
}
