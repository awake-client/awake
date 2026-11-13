package httpclient

import (
	"strings"

	"github.com/awake-client/awake/src/domain"
)

// applyEnvironmentAuthentication overrides the authentication type and the
// OAuth 2 grant type of a request with the reserved variables of its
// environment, authType and grantType.
func applyEnvironmentAuthentication(request *domain.Request, layers []map[string]string) error {
	values := mergeLayers(layers)
	if strings.TrimSpace(values[domain.AuthTypeVariable]) == "" && strings.TrimSpace(values[domain.GrantTypeVariable]) == "" {
		return nil
	}

	authentication, err := domain.ApplyEnvironmentAuthentication(resolvedAuthentication(request.Authentication), values)
	if err != nil {
		return err
	}
	request.Authentication = &authentication
	return nil
}
