package oauth2

import (
	"net/http"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/variables"
)

// Source supplies the authentication of an entry and its variable layers to
// the authorization flow. The application layer implements it so this feature
// does not read the workspace itself.
type Source interface {
	// ResolveAuthentication returns the authentication used by a request,
	// collection or folder, and the path of the entry that defines it: the
	// entry itself, or the folder or collection it inherits from. Tokens are
	// cached per defining entry, so requests inheriting one share its token.
	ResolveAuthentication(workspaceRoot string, entryPath string) (domain.Authentication, string, error)
	// LoadVariableLayers returns the variables visible to a request, or to a
	// collection or folder when given its directory path.
	LoadVariableLayers(workspaceRoot string, entryPath string, environmentFileName string) ([]variables.Layer, error)
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
	return Provider{manager: service.manager, source: service.source}
}

// Authorize runs the flow configured for an entry: a request, or a collection
// or folder given by its directory path. For the authorization code flow it opens
// the browser and waits for the loopback callback. For client credentials it
// contacts the token endpoint directly.
func (service *Service) Authorize(workspaceRoot string, entryPath string, environmentFileName string) (Status, error) {
	configuration, key, err := service.loadConfiguration(workspaceRoot, entryPath, environmentFileName)
	if err != nil {
		return Status{}, err
	}

	switch configuration.GrantType {
	case domain.OAuth2GrantClientCredentials:
		if _, err := service.manager.fetchClientCredentials(key, configuration); err != nil {
			return Status{}, err
		}
	case domain.OAuth2GrantPassword:
		if _, err := service.manager.fetchPassword(key, configuration); err != nil {
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

// Status reports the cached token of an entry without contacting the token
// endpoint.
func (service *Service) Status(workspaceRoot string, entryPath string, environmentFileName string) (Status, error) {
	configuration, key, err := service.loadConfiguration(workspaceRoot, entryPath, environmentFileName)
	if err != nil {
		return Status{}, err
	}

	return service.manager.status(key, configuration), nil
}

// Token returns the cached tokens of an entry so the user can inspect or copy
// them. It does not contact the token endpoint and returns
// ErrAuthorizationRequired when no token is cached.
func (service *Service) Token(workspaceRoot string, entryPath string, environmentFileName string) (TokenDetails, error) {
	configuration, key, err := service.loadConfiguration(workspaceRoot, entryPath, environmentFileName)
	if err != nil {
		return TokenDetails{}, err
	}

	return service.manager.details(key, configuration)
}

// Clear forgets the cached token of an entry, shared with the requests that
// inherit its authentication.
func (service *Service) Clear(workspaceRoot string, entryPath string, environmentFileName string) error {
	_, owner, err := service.source.ResolveAuthentication(workspaceRoot, entryPath)
	if err != nil {
		return err
	}
	service.manager.clear(tokenKey(workspaceRoot, owner, environmentFileName))
	return nil
}

func (service *Service) loadConfiguration(workspaceRoot string, entryPath string, environmentFileName string) (configuration, string, error) {
	authentication, owner, err := service.source.ResolveAuthentication(workspaceRoot, entryPath)
	if err != nil {
		return configuration{}, "", err
	}
	if authentication.Type != domain.AuthenticationOAuth2 {
		return configuration{}, "", ErrInvalidConfiguration
	}

	labeledLayers, err := service.source.LoadVariableLayers(workspaceRoot, entryPath, environmentFileName)
	if err != nil {
		return configuration{}, "", err
	}
	layers := variables.Maps(labeledLayers)

	resolved, err := resolveConfiguration(authentication, layers)
	if err != nil {
		return configuration{}, "", err
	}

	return resolved, tokenKey(workspaceRoot, owner, environmentFileName), nil
}

// tokenKey identifies a cached token by the entry that defines the
// authentication, so a collection and the requests inheriting from it share
// one token.
func tokenKey(workspaceRoot string, ownerPath string, environmentFileName string) string {
	return workspaceRoot + "\x00" + ownerPath + "\x00" + environmentFileName
}

// ownerKey returns the token key of a request, falling back to the request
// itself when its authentication cannot be resolved.
func ownerKey(source Source, workspaceRoot string, requestPath string, environmentFileName string) string {
	owner := requestPath
	if source != nil {
		if _, resolved, err := source.ResolveAuthentication(workspaceRoot, requestPath); err == nil && resolved != "" {
			owner = resolved
		}
	}
	return tokenKey(workspaceRoot, owner, environmentFileName)
}

// Provider supplies OAuth 2 access tokens to the HTTP client.
type Provider struct {
	manager *manager
	source  Source
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
		ownerKey(provider.source, workspaceRoot, requestPath, environmentFileName),
		configuration,
	)
}

// ExpireAccessToken marks the token of a request as expired, keeping its
// refresh token. The HTTP client calls it when the API answers 401 with a
// token believed valid, then asks for a fresh one.
func (provider Provider) ExpireAccessToken(workspaceRoot string, requestPath string, environmentFileName string) {
	provider.manager.expire(ownerKey(provider.source, workspaceRoot, requestPath, environmentFileName))
}
