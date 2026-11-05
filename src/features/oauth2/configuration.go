package oauth2

import (
	"fmt"
	"strings"

	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/shared/variables"
)

// configuration is the resolved OAuth 2 settings of one request.
type configuration struct {
	GrantType        domain.OAuth2GrantType
	AuthorizationURL string
	TokenURL         string
	ClientID         string
	ClientSecret     string
	Scopes           []string
	RedirectURL      string
	Audience         string
}

func resolveConfiguration(authentication domain.Authentication, layers []map[string]string) (configuration, error) {
	if authentication.Type != domain.AuthenticationOAuth2 {
		return configuration{}, ErrInvalidConfiguration
	}

	grantType := authentication.GrantType
	if grantType == "" && authentication.Token != "" {
		return configuration{}, ErrInvalidConfiguration
	}
	if grantType == "" {
		grantType = domain.OAuth2GrantAuthorizationCode
	}

	resolved := configuration{
		GrantType:    grantType,
		RedirectURL:  authentication.RedirectURL,
		ClientSecret: authentication.ClientSecret,
	}
	if err := interpolateInto(&resolved.AuthorizationURL, authentication.AuthorizationURL, layers); err != nil {
		return configuration{}, err
	}
	if err := interpolateInto(&resolved.TokenURL, authentication.TokenURL, layers); err != nil {
		return configuration{}, err
	}
	if err := interpolateInto(&resolved.ClientID, authentication.ClientID, layers); err != nil {
		return configuration{}, err
	}
	if err := interpolateInto(&resolved.ClientSecret, authentication.ClientSecret, layers); err != nil {
		return configuration{}, err
	}
	if err := interpolateInto(&resolved.RedirectURL, authentication.RedirectURL, layers); err != nil {
		return configuration{}, err
	}
	if err := interpolateInto(&resolved.Audience, authentication.Audience, layers); err != nil {
		return configuration{}, err
	}

	resolved.Scopes = make([]string, 0, len(authentication.Scopes))
	for _, scope := range authentication.Scopes {
		value, err := variables.Interpolate(scope, layers...)
		if err != nil {
			return configuration{}, err
		}
		if value != "" {
			resolved.Scopes = append(resolved.Scopes, value)
		}
	}

	if err := resolved.validate(); err != nil {
		return configuration{}, err
	}

	return resolved, nil
}

func interpolateInto(target *string, template string, layers []map[string]string) error {
	if template == "" {
		*target = ""
		return nil
	}

	value, err := variables.Interpolate(template, layers...)
	if err != nil {
		return err
	}
	*target = value
	return nil
}

func (configuration configuration) validate() error {
	switch configuration.GrantType {
	case domain.OAuth2GrantAuthorizationCode:
		if strings.TrimSpace(configuration.AuthorizationURL) == "" {
			return fmt.Errorf("%w: authorization URL is required", ErrInvalidConfiguration)
		}
	case domain.OAuth2GrantClientCredentials:
	default:
		return fmt.Errorf("%w: unsupported grant type", ErrInvalidConfiguration)
	}

	if strings.TrimSpace(configuration.TokenURL) == "" {
		return fmt.Errorf("%w: token URL is required", ErrInvalidConfiguration)
	}
	if strings.TrimSpace(configuration.ClientID) == "" {
		return fmt.Errorf("%w: client identifier is required", ErrInvalidConfiguration)
	}

	return nil
}

// fingerprint identifies the settings that produced a cached token, so a
// configuration change never reuses a token from another client or endpoint.
func (configuration configuration) fingerprint() string {
	return strings.Join([]string{
		string(configuration.GrantType),
		configuration.TokenURL,
		configuration.ClientID,
	}, "\x00")
}
