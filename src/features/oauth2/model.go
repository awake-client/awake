// Package oauth2 obtains and caches OAuth 2 access tokens for requests. The
// authorization code flow uses a loopback redirect with PKCE; the client
// credentials flow exchanges the client credentials directly.
package oauth2

import (
	"errors"
	"time"

	"github.com/clementherve/awake/src/domain"
)

var (
	// ErrAuthorizationRequired is returned when no usable token is cached and
	// the request needs an interactive authorization.
	ErrAuthorizationRequired = errors.New("oauth2: authorization required")
	// ErrInvalidConfiguration is returned when the request does not describe a
	// complete OAuth 2 configuration.
	ErrInvalidConfiguration = errors.New("oauth2: invalid configuration")
	// ErrTokenRequestFailed is returned when the token endpoint refuses the
	// exchange or answers with an unreadable payload.
	ErrTokenRequestFailed = errors.New("oauth2: token request failed")
	// ErrCallbackStateMismatch is returned when the redirect state does not
	// match the value generated for the authorization request.
	ErrCallbackStateMismatch = errors.New("oauth2: callback state mismatch")
)

const (
	// authorizationTimeout bounds the wait for the browser callback.
	authorizationTimeout = 5 * time.Minute
	// tokenRequestTimeout bounds one call to the token endpoint.
	tokenRequestTimeout = 30 * time.Second
	// tokenResponseLimit bounds the token endpoint response.
	tokenResponseLimit int64 = 1 << 20
	// tokenRefreshMargin refreshes a token shortly before it expires.
	tokenRefreshMargin = 30 * time.Second
)

// Status describes the cached token of one request.
type Status struct {
	GrantType             domain.OAuth2GrantType
	HasToken              bool
	ExpiresAtMilliseconds int64
	Scopes                []string
}

// Token is an access token and the data needed to refresh it.
type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scopes       []string
	ExpiresAt    time.Time
}

func (token Token) isExpired(now time.Time) bool {
	if token.ExpiresAt.IsZero() {
		return false
	}
	return !token.ExpiresAt.After(now.Add(tokenRefreshMargin))
}
