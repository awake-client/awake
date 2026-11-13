package domain

import (
	"fmt"
	"strings"
)

// Reserved environment variables that choose how a request authenticates when
// it is sent. They let one request, or one collection, switch between
// environments without editing its file: the credential fields stay in the
// request and may reference ordinary variables.
const (
	AuthTypeVariable  = "authType"
	GrantTypeVariable = "grantType"
)

// ApplyEnvironmentAuthentication returns the authentication with its type and
// OAuth 2 grant type replaced by the reserved variables, when they are set.
// Other variables, such as a token or a client identifier, reach the
// credential fields through the usual interpolation.
func ApplyEnvironmentAuthentication(authentication Authentication, values map[string]string) (Authentication, error) {
	if rawType := strings.TrimSpace(values[AuthTypeVariable]); rawType != "" {
		authenticationType, err := ParseAuthenticationType(rawType)
		if err != nil {
			return Authentication{}, err
		}
		authentication.Type = authenticationType
	}
	if rawGrant := strings.TrimSpace(values[GrantTypeVariable]); rawGrant != "" {
		grantType, err := ParseOAuth2GrantType(rawGrant)
		if err != nil {
			return Authentication{}, err
		}
		authentication.GrantType = grantType
	}
	return authentication, nil
}

// ParseAuthenticationType reads the authType variable, accepting the spelling
// of the request editor and the common variants.
func ParseAuthenticationType(value string) (AuthenticationType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "noauth":
		return AuthenticationNone, nil
	case "basic":
		return AuthenticationBasic, nil
	case "bearer":
		return AuthenticationBearer, nil
	case "apikey", "api-key", "api_key":
		return AuthenticationAPIKey, nil
	case "oauth2":
		return AuthenticationOAuth2, nil
	default:
		return "", fmt.Errorf("%s %q: use basic, bearer, apiKey, oauth2 or none", AuthTypeVariable, value)
	}
}

// ParseOAuth2GrantType reads the grantType variable, accepting both the grant
// names of the request editor and those of imported collections.
func ParseOAuth2GrantType(value string) (OAuth2GrantType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "authorizationcode", "authorization_code", "authorization-code":
		return OAuth2GrantAuthorizationCode, nil
	case "clientcredentials", "client_credentials", "client_credential":
		return OAuth2GrantClientCredentials, nil
	case "password", "resource_owner_password", "resource_owner_password_credentials":
		return OAuth2GrantPassword, nil
	default:
		return "", fmt.Errorf("%s %q: use authorizationCode, clientCredentials or password", GrantTypeVariable, value)
	}
}
