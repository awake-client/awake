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
	"unicode/utf8"

	"github.com/awake-client/awake/src/domain"
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

	// An expired token is refreshed silently: with its refresh token when the
	// server issued one, otherwise by asking again for the grants that need
	// no user interaction.
	if found && cached.fingerprint == configuration.fingerprint() && cached.token.RefreshToken != "" {
		token, err := manager.refresh(key, configuration, cached.token.RefreshToken)
		if err == nil {
			return token.AccessToken, nil
		}
		manager.clear(key)
	}

	switch configuration.GrantType {
	case domain.OAuth2GrantClientCredentials:
		token, err := manager.fetchClientCredentials(key, configuration)
		if err != nil {
			return "", err
		}
		return token.AccessToken, nil

	case domain.OAuth2GrantPassword:
		token, err := manager.fetchPassword(key, configuration)
		if err != nil {
			return "", err
		}
		return token.AccessToken, nil

	default:
		// The authorization code flow needs the user in the browser.
		return "", ErrAuthorizationRequired
	}
}

// expire marks the cached token of a key as expired while keeping its refresh
// token, so the next request refreshes it. It is used when the API rejects a
// token the client still believed valid.
func (manager *manager) expire(key string) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	cached, found := manager.tokens[key]
	if !found {
		return
	}
	cached.token.ExpiresAt = manager.now().Add(-time.Second)
	manager.tokens[key] = cached
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

// details returns the cached tokens of a key, or ErrAuthorizationRequired when
// none matches the current configuration.
func (manager *manager) details(key string, configuration configuration) (TokenDetails, error) {
	manager.mutex.Lock()
	cached, found := manager.tokens[key]
	manager.mutex.Unlock()

	if !found || cached.fingerprint != configuration.fingerprint() {
		return TokenDetails{}, ErrAuthorizationRequired
	}

	details := TokenDetails{
		GrantType:    configuration.GrantType,
		AccessToken:  cached.token.AccessToken,
		TokenType:    cached.token.TokenType,
		RefreshToken: cached.token.RefreshToken,
		IDToken:      cached.token.IDToken,
		Scopes:       append([]string{}, cached.token.Scopes...),
	}
	if !cached.token.ExpiresAt.IsZero() {
		details.ExpiresAtMilliseconds = cached.token.ExpiresAt.UnixMilli()
	}

	return details, nil
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

// fetchPassword exchanges the user name and password for a token.
func (manager *manager) fetchPassword(key string, configuration configuration) (Token, error) {
	token, err := manager.requestToken(configuration, url.Values{
		"grant_type": {"password"},
		"username":   {configuration.Username},
		"password":   {configuration.Password},
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
	if token.IDToken == "" {
		manager.mutex.Lock()
		token.IDToken = manager.tokens[key].token.IDToken
		manager.mutex.Unlock()
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
		return Token{}, tokenEndpointError(configuration.TokenURL, response.StatusCode, body)
	}

	return parseToken(body, manager.now(), configuration.Scopes)
}

type tokenPayload struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
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
		IDToken:      payload.IDToken,
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

// tokenEndpointError describes a refused token request with what the user
// needs to fix it: the endpoint called, the HTTP status and the reason given by
// the server, either an OAuth 2 error or the start of a plain text body.
func tokenEndpointError(tokenURL string, statusCode int, body []byte) error {
	reason := fmt.Sprintf("token endpoint %s responded %d %s", displayURL(tokenURL), statusCode, http.StatusText(statusCode))

	var payload tokenPayload
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		detail := payload.Error
		if payload.ErrorDescription != "" {
			detail += ": " + payload.ErrorDescription
		}
		return fmt.Errorf("%w: %s: %s", ErrTokenRequestFailed, reason, detail)
	}

	if text := bodyExcerpt(body); text != "" {
		return fmt.Errorf("%w: %s: %s", ErrTokenRequestFailed, reason, text)
	}
	return fmt.Errorf("%w: %s", ErrTokenRequestFailed, reason)
}

// displayURL drops the credentials and the query of a URL before it is shown.
func displayURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// bodyExcerpt returns the start of a plain text error body on one line, or
// nothing for an HTML page or a binary payload.
func bodyExcerpt(body []byte) string {
	const limit = 200
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" || strings.HasPrefix(text, "<") || !utf8.ValidString(text) {
		return ""
	}
	if len(text) > limit {
		text = text[:limit] + "…"
	}
	return text
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
