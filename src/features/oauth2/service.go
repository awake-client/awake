package oauth2

import (
	"net/http"

	"github.com/clementherve/awake/src/domain"
)

// Source supplies the request and its variable layers to the authorization
// flow. The application layer implements it so this feature does not read the
// workspace itself.
type Source interface {
	LoadRequest(workspaceRoot string, relativePath string) (domain.Request, error)
	LoadVariableLayers(workspaceRoot string, environmentFileName string) ([]map[string]string, error)
}

// Service exposes the OAuth 2 flows to the application layer.
type Service struct {
	source  Source
	manager *manager
	browser Browser
}

// NewService returns an OAuth 2 service that opens the system browser.
func NewService(source Source) *Service {
	return NewServiceWithDependencies(
		source,
		systemBrowser{},
		&http.Client{Timeout: tokenRequestTimeout},
	)
}

// NewServiceWithDependencies returns a service with an injected browser and
// HTTP client. It exists so tests can avoid the network and the browser.
func NewServiceWithDependencies(source Source, browser Browser, client *http.Client) *Service {
	return &Service{
		source:  source,
		manager: newManager(client),
		browser: browser,
	}
}

// NewProvider returns the token source used by the HTTP client. It is a
// package function rather than a service method so the token cache stays out
// of the Wails bindings.
func NewProvider(service *Service) Provider {
	return Provider{manager: service.manager}
}

// Authorize runs the configured flow. For the authorization code flow it opens
// the browser and waits for the loopback callback. For client credentials it
// contacts the token endpoint directly.
func (service *Service) Authorize(workspaceRoot string, requestPath string, environmentFileName string) (Status, error) {
	configuration, key, err := service.loadConfiguration(workspaceRoot, requestPath, environmentFileName)
	if err != nil {
		return Status{}, err
	}

	switch configuration.GrantType {
	case domain.OAuth2GrantClientCredentials:
		if _, err := service.manager.fetchClientCredentials(key, configuration); err != nil {
			return Status{}, err
		}
	case domain.OAuth2GrantAuthorizationCode:
		if _, err := service.manager.authorizeCode(key, configuration, service.browser); err != nil {
			return Status{}, err
		}
	default:
		return Status{}, ErrInvalidConfiguration
	}

	return service.manager.status(key, configuration), nil
}

// Status reports the cached token of a request without contacting the token
// endpoint.
func (service *Service) Status(workspaceRoot string, requestPath string, environmentFileName string) (Status, error) {
	configuration, key, err := service.loadConfiguration(workspaceRoot, requestPath, environmentFileName)
	if err != nil {
		return Status{}, err
	}

	return service.manager.status(key, configuration), nil
}

// Clear forgets the cached token of a request.
func (service *Service) Clear(workspaceRoot string, requestPath string, environmentFileName string) error {
	service.manager.clear(tokenKey(workspaceRoot, requestPath, environmentFileName))
	return nil
}

func (service *Service) loadConfiguration(workspaceRoot string, requestPath string, environmentFileName string) (configuration, string, error) {
	request, err := service.source.LoadRequest(workspaceRoot, requestPath)
	if err != nil {
		return configuration{}, "", err
	}
	if request.Authentication == nil || request.Authentication.Type != domain.AuthenticationOAuth2 {
		return configuration{}, "", ErrInvalidConfiguration
	}

	layers, err := service.source.LoadVariableLayers(workspaceRoot, environmentFileName)
	if err != nil {
		return configuration{}, "", err
	}

	resolved, err := resolveConfiguration(*request.Authentication, layers)
	if err != nil {
		return configuration{}, "", err
	}

	return resolved, tokenKey(workspaceRoot, requestPath, environmentFileName), nil
}

func tokenKey(workspaceRoot string, requestPath string, environmentFileName string) string {
	return workspaceRoot + "\x00" + requestPath + "\x00" + environmentFileName
}

// Provider supplies OAuth 2 access tokens to the HTTP client.
type Provider struct {
	manager *manager
}

// AccessToken returns a cached or freshly obtained token for a request. It
// returns ErrAuthorizationRequired when the authorization code flow needs the
// user to open a browser first.
func (provider Provider) AccessToken(
	workspaceRoot string,
	requestPath string,
	environmentFileName string,
	authentication domain.Authentication,
	layers []map[string]string,
) (string, error) {
	configuration, err := resolveConfiguration(authentication, layers)
	if err != nil {
		return "", err
	}

	return provider.manager.accessToken(
		tokenKey(workspaceRoot, requestPath, environmentFileName),
		configuration,
	)
}
