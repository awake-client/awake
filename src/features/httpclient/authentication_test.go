package httpclient

import (
	"testing"

	"github.com/awake-client/awake/src/domain"
)

func TestAuthenticationTypeFromAcceptsEditorAndLowercaseSpellings(t *testing.T) {
	cases := map[string]domain.AuthenticationType{
		"none":   domain.AuthenticationNone,
		"basic":  domain.AuthenticationBasic,
		"bearer": domain.AuthenticationBearer,
		"apikey": domain.AuthenticationAPIKey,
		"apiKey": domain.AuthenticationAPIKey,
		"oauth2": domain.AuthenticationOAuth2,
	}
	for value, expected := range cases {
		actual, err := authenticationTypeFrom(value)
		if err != nil || actual != expected {
			t.Errorf("%q: expected %q, got %q (%v)", value, expected, actual, err)
		}
	}

	if _, err := authenticationTypeFrom("token"); err == nil {
		t.Error("expected an unsupported authentication type to fail")
	}
}

func TestGrantTypeFromAcceptsEditorAndImportedSpellings(t *testing.T) {
	cases := map[string]domain.OAuth2GrantType{
		"authorizationCode":                   domain.OAuth2GrantAuthorizationCode,
		"authorization_code":                  domain.OAuth2GrantAuthorizationCode,
		"clientCredentials":                   domain.OAuth2GrantClientCredentials,
		"client_credentials":                  domain.OAuth2GrantClientCredentials,
		"client_credential":                   domain.OAuth2GrantClientCredentials,
		"password":                            domain.OAuth2GrantPassword,
		"resource_owner_password_credentials": domain.OAuth2GrantPassword,
	}
	for value, expected := range cases {
		actual, err := grantTypeFrom(value)
		if err != nil || actual != expected {
			t.Errorf("%q: expected %q, got %q (%v)", value, expected, actual, err)
		}
	}

	if _, err := grantTypeFrom("implicit"); err == nil {
		t.Error("expected an unsupported grant type to fail")
	}
}
