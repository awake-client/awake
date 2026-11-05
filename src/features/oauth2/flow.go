package oauth2

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type callbackResult struct {
	code             string
	state            string
	errorDescription string
}

func (manager *manager) authorizeCode(key string, configuration configuration, browser Browser) (Token, error) {
	listener, redirectURL, callbackPath, err := listenForCallback(configuration.RedirectURL)
	if err != nil {
		return Token{}, err
	}

	codeVerifier, err := randomURLSafeString(32)
	if err != nil {
		listener.Close()
		return Token{}, err
	}
	state, err := randomURLSafeString(24)
	if err != nil {
		listener.Close()
		return Token{}, err
	}

	challengeHash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(challengeHash[:])

	authorizationURL, err := buildAuthorizationURL(configuration, redirectURL, state, codeChallenge)
	if err != nil {
		listener.Close()
		return Token{}, err
	}

	callbacks := make(chan callbackResult, 1)
	server := &http.Server{
		Handler: callbackHandler(callbackPath, callbacks),
	}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		_ = server.Close()
	}()

	if err := browser.Open(authorizationURL); err != nil {
		return Token{}, fmt.Errorf("open authorization page: %w", err)
	}

	var callback callbackResult
	select {
	case callback = <-callbacks:
	case <-time.After(authorizationTimeout):
		return Token{}, fmt.Errorf("%w: authorization timed out", ErrAuthorizationRequired)
	}

	if callback.errorDescription != "" {
		return Token{}, fmt.Errorf("%w: %s", ErrAuthorizationRequired, callback.errorDescription)
	}
	if callback.state != state {
		return Token{}, ErrCallbackStateMismatch
	}
	if callback.code == "" {
		return Token{}, fmt.Errorf("%w: no authorization code", ErrAuthorizationRequired)
	}

	token, err := manager.requestToken(configuration, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {callback.code},
		"redirect_uri":  {redirectURL},
		"code_verifier": {codeVerifier},
	})
	if err != nil {
		return Token{}, err
	}

	manager.store(key, configuration, token)
	return token, nil
}

func listenForCallback(rawRedirectURL string) (net.Listener, string, string, error) {
	host := "127.0.0.1"
	path := "/callback"
	port := "0"

	var parsed *url.URL
	if rawRedirectURL != "" {
		var err error
		parsed, err = url.Parse(rawRedirectURL)
		if err != nil {
			return nil, "", "", fmt.Errorf("%w: invalid redirect URL", ErrInvalidConfiguration)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, "", "", fmt.Errorf("%w: the redirect URL must use http", ErrInvalidConfiguration)
		}
		if !isLoopbackHost(parsed.Hostname()) {
			return nil, "", "", fmt.Errorf("%w: the redirect URL must use a loopback host", ErrInvalidConfiguration)
		}
		host = parsed.Hostname()
		if parsed.Path != "" {
			path = parsed.Path
		}
		if parsed.Port() != "" {
			port = parsed.Port()
		}
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, "", "", fmt.Errorf("listen for authorization callback: %w", err)
	}

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		listener.Close()
		return nil, "", "", fmt.Errorf("listen for authorization callback: unexpected address")
	}

	redirect := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, fmt.Sprintf("%d", address.Port)),
		Path:   path,
	}
	return listener, redirect.String(), path, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func buildAuthorizationURL(configuration configuration, redirectURL string, state string, codeChallenge string) (string, error) {
	parsed, err := url.Parse(configuration.AuthorizationURL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid authorization URL", ErrInvalidConfiguration)
	}

	query := parsed.Query()
	query.Set("response_type", "code")
	query.Set("client_id", configuration.ClientID)
	query.Set("redirect_uri", redirectURL)
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	if len(configuration.Scopes) > 0 {
		query.Set("scope", strings.Join(configuration.Scopes, " "))
	}
	if configuration.Audience != "" {
		query.Set("audience", configuration.Audience)
	}
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func callbackHandler(path string, callbacks chan<- callbackResult) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != path {
			http.NotFound(writer, request)
			return
		}

		result := callbackResult{
			code:  request.URL.Query().Get("code"),
			state: request.URL.Query().Get("state"),
		}
		if errorCode := request.URL.Query().Get("error"); errorCode != "" {
			description := request.URL.Query().Get("error_description")
			if description == "" {
				description = errorCode
			}
			result.errorDescription = description
		}

		select {
		case callbacks <- result:
		default:
		}

		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(callbackPage))
	})
}

const callbackPage = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Awake authorization</title></head>
<body style="font-family: system-ui, sans-serif; padding: 2rem;">
<p>Authorization received. You can close this window and return to Awake.</p>
</body>
</html>
`

func randomURLSafeString(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
