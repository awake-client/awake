package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/clementherve/awake/src/domain"
)

type cachedToken struct {
	token       Token
	fingerprint string
}

// manager caches tokens per request and talks to the token endpoint.
type manager struct {
	mutex  sync.Mutex
	tokens map[string]cachedToken

	client *http.Client
	now    func() time.Time
}

func newManager(client *http.Client) *manager {
	return &manager{
		tokens: make(map[string]cachedToken),
		client: client,
		now:    time.Now,
	}
}

func (manager *manager) accessToken(key string, configuration configuration) (string, error) {
	manager.mutex.Lock()
	cached, found := manager.tokens[key]
	manager.mutex.Unlock()

	now := manager.now()
	if found && cached.fingerprint == configuration.fingerprint() && !cached.token.isExpired(now) {
		return cached.token.AccessToken, nil
	}

	switch configuration.GrantType {
	case domain.OAuth2GrantClientCredentials:
		token, err := manager.fetchClientCredentials(key, configuration)
		if err != nil {
			return "", err
		}
		return token.AccessToken, nil

	case domain.OAuth2GrantAuthorizationCode:
		if found && cached.fingerprint == configuration.fingerprint() && cached.token.RefreshToken != "" {
			token, err := manager.refresh(key, configuration, cached.token.RefreshToken)
			if err == nil {
				return token.AccessToken, nil
			}
			manager.clear(key)
		}
		return "", ErrAuthorizationRequired

	default:
		return "", ErrAuthorizationRequired
	}
}

func (manager *manager) status(key string, configuration configuration) Status {
	manager.mutex.Lock()
	cached, found := manager.tokens[key]
	manager.mutex.Unlock()

	status := Status{GrantType: configuration.GrantType}
	if !found || cached.fingerprint != configuration.fingerprint() {
		return status
	}

	status.HasToken = true
	status.Scopes = append([]string{}, cached.token.Scopes...)
	if !cached.token.ExpiresAt.IsZero() {
		status.ExpiresAtMilliseconds = cached.token.ExpiresAt.UnixMilli()
	}

	return status
}

func (manager *manager) clear(key string) {
	manager.mutex.Lock()
	delete(manager.tokens, key)
	manager.mutex.Unlock()
}

func (manager *manager) store(key string, configuration configuration, token Token) {
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if len(token.Scopes) == 0 {
		token.Scopes = append([]string{}, configuration.Scopes...)
	}

	manager.mutex.Lock()
	manager.tokens[key] = cachedToken{
		token:       token,
		fingerprint: configuration.fingerprint(),
	}
	manager.mutex.Unlock()
}

func (manager *manager) fetchClientCredentials(key string, configuration configuration) (Token, error) {
	token, err := manager.requestToken(configuration, url.Values{
		"grant_type": {"client_credentials"},
	})
	if err != nil {
		return Token{}, err
	}

	manager.store(key, configuration, token)
	return token, nil
}

func (manager *manager) refresh(key string, configuration configuration, refreshToken string) (Token, error) {
	token, err := manager.requestToken(configuration, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return Token{}, err
	}

	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	manager.store(key, configuration, token)
	return token, nil
}

func (manager *manager) requestToken(configuration configuration, form url.Values) (Token, error) {
	form.Set("client_id", configuration.ClientID)
	if configuration.ClientSecret != "" {
		form.Set("client_secret", configuration.ClientSecret)
	}

	requestContext, cancel := context.WithTimeout(context.Background(), tokenRequestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodPost,
		configuration.TokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return Token{}, fmt.Errorf("build token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := manager.client.Do(request)
	if err != nil {
		return Token{}, fmt.Errorf("request token: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, tokenResponseLimit))
	if err != nil {
		return Token{}, fmt.Errorf("read token response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Token{}, tokenEndpointError(response.StatusCode, body)
	}

	return parseToken(body, manager.now(), configuration.Scopes)
}

type tokenPayload struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func parseToken(body []byte, now time.Time, defaultScopes []string) (Token, error) {
	var payload tokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Token{}, fmt.Errorf("%w: invalid JSON response", ErrTokenRequestFailed)
	}
	if payload.Error != "" {
		return Token{}, tokenError(payload.Error, payload.ErrorDescription)
	}
	if payload.AccessToken == "" {
		return Token{}, fmt.Errorf("%w: no access token", ErrTokenRequestFailed)
	}

	token := Token{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		TokenType:    payload.TokenType,
		Scopes:       splitScopes(payload.Scope),
	}
	if len(token.Scopes) == 0 {
		token.Scopes = append([]string{}, defaultScopes...)
	}
	if payload.ExpiresIn > 0 {
		token.ExpiresAt = now.Add(time.Duration(payload.ExpiresIn) * time.Second)
	}

	return token, nil
}

func tokenEndpointError(statusCode int, body []byte) error {
	var payload tokenPayload
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		return tokenError(payload.Error, payload.ErrorDescription)
	}

	return fmt.Errorf("%w: server responded %d", ErrTokenRequestFailed, statusCode)
}

func tokenError(code string, description string) error {
	if description == "" {
		return fmt.Errorf("%w: %s", ErrTokenRequestFailed, code)
	}
	return fmt.Errorf("%w: %s: %s", ErrTokenRequestFailed, code, description)
}

func splitScopes(scope string) []string {
	fields := strings.Fields(scope)
	if len(fields) == 0 {
		return nil
	}
	return fields
}
