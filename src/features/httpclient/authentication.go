package httpclient

import (
	"fmt"
	"strings"

	"github.com/awake-client/awake/src/domain"
)

// Reserved environment variables that choose how a request authenticates when
// it is sent. They let one request, or one collection, switch between
// environments without editing its file: the credential fields stay in the
// request and may reference ordinary variables.
const (
	authTypeVariable  = "authType"
	grantTypeVariable = "grantType"
)

// applyEnvironmentAuthentication overrides the authentication type and the
// OAuth 2 grant type of a request with the reserved variables of its
// environment. Other variables, such as a token or a client identifier, reach
// the credential fields through the usual interpolation.
func applyEnvironmentAuthentication(request *domain.Request, layers []map[string]string) error {
	values := mergeLayers(layers)
	rawType := strings.TrimSpace(values[authTypeVariable])
	rawGrant := strings.TrimSpace(values[grantTypeVariable])
	if rawType == "" && rawGrant == "" {
		return nil
	}

	authentication := resolvedAuthentication(request.Authentication)
	if rawType != "" {
		authenticationType, err := authenticationTypeFrom(rawType)
		if err != nil {
			return err
		}
		authentication.Type = authenticationType
	}
	if rawGrant != "" {
		grantType, err := grantTypeFrom(rawGrant)
		if err != nil {
			return err
		}
		authentication.GrantType = grantType
	}

	request.Authentication = &authentication
	return nil
}

// authenticationTypeFrom reads the authType variable, accepting the spelling
// of the request editor and the common variants.
func authenticationTypeFrom(value string) (domain.AuthenticationType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "noauth":
		return domain.AuthenticationNone, nil
	case "basic":
		return domain.AuthenticationBasic, nil
	case "bearer":
		return domain.AuthenticationBearer, nil
	case "apikey", "api-key", "api_key":
		return domain.AuthenticationAPIKey, nil
	case "oauth2":
		return domain.AuthenticationOAuth2, nil
	default:
		return "", fmt.Errorf("%s %q: use basic, bearer, apiKey, oauth2 or none", authTypeVariable, value)
	}
}

// grantTypeFrom reads the grantType variable, accepting both the grant names
// of the request editor and those of imported collections.
func grantTypeFrom(value string) (domain.OAuth2GrantType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "authorizationcode", "authorization_code", "authorization-code":
		return domain.OAuth2GrantAuthorizationCode, nil
	case "clientcredentials", "client_credentials", "client_credential":
		return domain.OAuth2GrantClientCredentials, nil
	case "password", "resource_owner_password", "resource_owner_password_credentials":
		return domain.OAuth2GrantPassword, nil
	default:
		return "", fmt.Errorf("%s %q: use authorizationCode, clientCredentials or password", grantTypeVariable, value)
	}
}
