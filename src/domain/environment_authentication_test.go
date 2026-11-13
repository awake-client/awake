package domain

import "testing"

func TestParseAuthenticationTypeAcceptsEditorAndLowercaseSpellings(t *testing.T) {
	cases := map[string]AuthenticationType{
		"none":   AuthenticationNone,
		"basic":  AuthenticationBasic,
		"bearer": AuthenticationBearer,
		"apikey": AuthenticationAPIKey,
		"apiKey": AuthenticationAPIKey,
		"oauth2": AuthenticationOAuth2,
	}
	for value, expected := range cases {
		actual, err := ParseAuthenticationType(value)
		if err != nil || actual != expected {
			t.Errorf("%q: expected %q, got %q (%v)", value, expected, actual, err)
		}
	}

	if _, err := ParseAuthenticationType("token"); err == nil {
		t.Error("expected an unsupported authentication type to fail")
	}
}

func TestParseOAuth2GrantTypeAcceptsEditorAndImportedSpellings(t *testing.T) {
	cases := map[string]OAuth2GrantType{
		"authorizationCode":                   OAuth2GrantAuthorizationCode,
		"authorization_code":                  OAuth2GrantAuthorizationCode,
		"clientCredentials":                   OAuth2GrantClientCredentials,
		"client_credentials":                  OAuth2GrantClientCredentials,
		"client_credential":                   OAuth2GrantClientCredentials,
		"password":                            OAuth2GrantPassword,
		"resource_owner_password_credentials": OAuth2GrantPassword,
	}
	for value, expected := range cases {
		actual, err := ParseOAuth2GrantType(value)
		if err != nil || actual != expected {
			t.Errorf("%q: expected %q, got %q (%v)", value, expected, actual, err)
		}
	}

	if _, err := ParseOAuth2GrantType("implicit"); err == nil {
		t.Error("expected an unsupported grant type to fail")
	}
}

func TestApplyEnvironmentAuthenticationOverridesTheTypeAndGrant(t *testing.T) {
	base := Authentication{Type: AuthenticationBearer, Token: "kept"}

	unchanged, err := ApplyEnvironmentAuthentication(base, map[string]string{"other": "x"})
	if err != nil || unchanged.Type != base.Type || unchanged.GrantType != base.GrantType || unchanged.Token != base.Token {
		t.Errorf("without reserved variables the authentication must not change, got %+v, %v", unchanged, err)
	}

	overridden, err := ApplyEnvironmentAuthentication(base, map[string]string{
		AuthTypeVariable:  "oauth2",
		GrantTypeVariable: "client_credentials",
	})
	if err != nil || overridden.Type != AuthenticationOAuth2 || overridden.GrantType != OAuth2GrantClientCredentials || overridden.Token != "kept" {
		t.Errorf("unexpected override: %+v, %v", overridden, err)
	}

	if _, err := ApplyEnvironmentAuthentication(base, map[string]string{AuthTypeVariable: "token"}); err == nil {
		t.Error("expected an unsupported authType to fail")
	}
}
