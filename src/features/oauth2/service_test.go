package oauth2

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/variables"
)

type fakeSource struct {
	request domain.Request
	layers  []map[string]string
}

func (source fakeSource) ResolveAuthentication(_ string, entryPath string) (domain.Authentication, string, error) {
	if source.request.Authentication == nil {
		return domain.Authentication{Type: domain.AuthenticationNone}, entryPath, nil
	}
	return *source.request.Authentication, entryPath, nil
}

func (source fakeSource) LoadVariableLayers(_ string, _ string, _ string) ([]variables.Layer, error) {
	return variables.Unlabeled(source.layers...), nil
}

type fakeBrowser struct {
	open func(rawURL string) error
}

func (browser fakeBrowser) Open(rawURL string) error {
	return browser.open(rawURL)
}

func clientCredentialsRequest(tokenURL string) domain.Request {
	return domain.Request{
		Name:   "List users",
		Method: domain.MethodGet,
		URL:    "https://example.com/users",
		Authentication: &domain.Authentication{
			Type:         domain.AuthenticationOAuth2,
			GrantType:    domain.OAuth2GrantClientCredentials,
			TokenURL:     tokenURL,
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			Scopes:       []string{"read", "write"},
		},
	}
}

func TestAuthorizeClientCredentialsStoresAndReusesToken(t *testing.T) {
	var requests int
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if request.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("unexpected grant type: %q", request.Form.Get("grant_type"))
		}
		if request.Form.Get("client_id") != "client-id" {
			t.Errorf("unexpected client id: %q", request.Form.Get("client_id"))
		}
		if request.Form.Get("client_secret") != "client-secret" {
			t.Errorf("unexpected client secret: %q", request.Form.Get("client_secret"))
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"token-1","token_type":"Bearer","expires_in":3600,"scope":"read"}`))
	}))
	defer tokenServer.Close()

	service := NewServiceWithDependencies(
		fakeSource{request: clientCredentialsRequest(tokenServer.URL)},
		fakeBrowser{},
		tokenServer.Client(),
	)

	status, err := service.Authorize("/workspace", "collection/request.yaml", "development")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !status.HasToken {
		t.Fatalf("expected a token: %+v", status)
	}
	if status.ExpiresAtMilliseconds == 0 {
		t.Errorf("expected an expiry: %+v", status)
	}

	token, err := NewProvider(service).AccessToken(
		"/workspace",
		"collection/request.yaml",
		"development",
		*clientCredentialsRequest(tokenServer.URL).Authentication,
		nil,
	)
	if err != nil {
		t.Fatalf("access token: %v", err)
	}
	if token != "token-1" {
		t.Errorf("unexpected token: %q", token)
	}
	if requests != 1 {
		t.Errorf("expected one token request, got %d", requests)
	}

	if err := service.Clear("/workspace", "collection/request.yaml", "development"); err != nil {
		t.Fatalf("clear token: %v", err)
	}
	status, err = service.Status("/workspace", "collection/request.yaml", "development")
	if err != nil {
		t.Fatalf("token status: %v", err)
	}
	if status.HasToken {
		t.Errorf("expected the token to be cleared: %+v", status)
	}
}

func TestAuthorizeAuthorizationCodeUsesPkce(t *testing.T) {
	var tokenRequest url.Values
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		tokenRequest = request.Form
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"code-token","token_type":"Bearer","expires_in":600}`))
	}))
	defer tokenServer.Close()

	request := domain.Request{
		Name:   "List users",
		Method: domain.MethodGet,
		URL:    "https://example.com/users",
		Authentication: &domain.Authentication{
			Type:             domain.AuthenticationOAuth2,
			GrantType:        domain.OAuth2GrantAuthorizationCode,
			AuthorizationURL: "https://example.com/authorize",
			TokenURL:         tokenServer.URL,
			ClientID:         "client-id",
			Scopes:           []string{"read"},
			Audience:         "https://api.example.com",
		},
	}

	var browserMutex sync.Mutex
	var authorizationURL *url.URL
	browser := fakeBrowser{open: func(rawURL string) error {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return err
		}

		browserMutex.Lock()
		authorizationURL = parsed
		browserMutex.Unlock()

		redirect := parsed.Query().Get("redirect_uri")
		state := parsed.Query().Get("state")
		go func() {
			_, _ = http.Get(fmt.Sprintf("%s?code=authorization-code&state=%s", redirect, url.QueryEscape(state)))
		}()
		return nil
	}}

	service := NewServiceWithDependencies(fakeSource{request: request}, browser, tokenServer.Client())
	status, err := service.Authorize("/workspace", "collection/request.yaml", "")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !status.HasToken {
		t.Fatalf("expected a token: %+v", status)
	}

	browserMutex.Lock()
	query := authorizationURL.Query()
	browserMutex.Unlock()

	if query.Get("response_type") != "code" {
		t.Errorf("unexpected response type: %q", query.Get("response_type"))
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Errorf("unexpected challenge method: %q", query.Get("code_challenge_method"))
	}
	if query.Get("code_challenge") == "" {
		t.Error("expected a code challenge")
	}
	if query.Get("scope") != "read" {
		t.Errorf("unexpected scope: %q", query.Get("scope"))
	}
	if query.Get("audience") != "https://api.example.com" {
		t.Errorf("unexpected audience: %q", query.Get("audience"))
	}

	if tokenRequest.Get("grant_type") != "authorization_code" {
		t.Errorf("unexpected grant type: %q", tokenRequest.Get("grant_type"))
	}
	if tokenRequest.Get("code") != "authorization-code" {
		t.Errorf("unexpected code: %q", tokenRequest.Get("code"))
	}
	if tokenRequest.Get("code_verifier") == "" {
		t.Error("expected a code verifier")
	}
	if tokenRequest.Get("redirect_uri") != query.Get("redirect_uri") {
		t.Errorf("redirect mismatch: %q and %q", tokenRequest.Get("redirect_uri"), query.Get("redirect_uri"))
	}
}

func TestAuthorizeAuthorizationCodeRejectsStateMismatch(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Error("the token endpoint must not be called")
	}))
	defer tokenServer.Close()

	request := authorizationCodeRequest(tokenServer.URL)
	browser := fakeBrowser{open: func(rawURL string) error {
		parsed, _ := url.Parse(rawURL)
		redirect := parsed.Query().Get("redirect_uri")
		go func() {
			_, _ = http.Get(redirect + "?code=authorization-code&state=wrong")
		}()
		return nil
	}}

	service := NewServiceWithDependencies(fakeSource{request: request}, browser, tokenServer.Client())
	_, err := service.Authorize("/workspace", "collection/request.yaml", "")
	if !errors.Is(err, ErrCallbackStateMismatch) {
		t.Fatalf("expected ErrCallbackStateMismatch, got %v", err)
	}
}

func TestProviderRequiresAuthorizationForAuthorizationCode(t *testing.T) {
	service := NewServiceWithDependencies(
		fakeSource{request: authorizationCodeRequest("https://example.com/token")},
		fakeBrowser{},
		http.DefaultClient,
	)

	_, err := NewProvider(service).AccessToken(
		"/workspace",
		"collection/request.yaml",
		"",
		*authorizationCodeRequest("https://example.com/token").Authentication,
		nil,
	)
	if !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("expected ErrAuthorizationRequired, got %v", err)
	}
}

func TestResolveConfigurationInterpolatesVariables(t *testing.T) {
	authentication := domain.Authentication{
		Type:         domain.AuthenticationOAuth2,
		GrantType:    domain.OAuth2GrantClientCredentials,
		TokenURL:     "{{identityUrl}}/token",
		ClientID:     "{{clientId}}",
		ClientSecret: "{{clientSecret}}",
		Scopes:       []string{"api.{{scope}}"},
	}
	layers := []map[string]string{{
		"identityUrl":  "https://identity.example.com",
		"clientId":     "awake",
		"clientSecret": "secret",
		"scope":        "read",
	}}

	configuration, err := resolveConfiguration(authentication, layers)
	if err != nil {
		t.Fatalf("resolve configuration: %v", err)
	}
	if configuration.TokenURL != "https://identity.example.com/token" {
		t.Errorf("unexpected token URL: %q", configuration.TokenURL)
	}
	if configuration.ClientSecret != "secret" {
		t.Errorf("unexpected client secret: %q", configuration.ClientSecret)
	}
	if len(configuration.Scopes) != 1 || configuration.Scopes[0] != "api.read" {
		t.Errorf("unexpected scopes: %v", configuration.Scopes)
	}
}

func TestResolveConfigurationRejectsIncompleteSettings(t *testing.T) {
	authentication := domain.Authentication{
		Type:      domain.AuthenticationOAuth2,
		GrantType: domain.OAuth2GrantClientCredentials,
		ClientID:  "client-id",
	}

	_, err := resolveConfiguration(authentication, nil)
	if !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("expected ErrInvalidConfiguration, got %v", err)
	}
	if !strings.Contains(err.Error(), "token URL") {
		t.Errorf("unexpected error: %v", err)
	}
}

func authorizationCodeRequest(tokenURL string) domain.Request {
	return domain.Request{
		Name:   "List users",
		Method: domain.MethodGet,
		URL:    "https://example.com/users",
		Authentication: &domain.Authentication{
			Type:             domain.AuthenticationOAuth2,
			GrantType:        domain.OAuth2GrantAuthorizationCode,
			AuthorizationURL: "https://example.com/authorize",
			TokenURL:         tokenURL,
			ClientID:         "client-id",
		},
	}
}

func TestPasswordGrantRefreshesSilently(t *testing.T) {
	var grants []string
	issued := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		grants = append(grants, request.Form.Get("grant_type"))
		if request.Form.Get("grant_type") == "password" &&
			(request.Form.Get("username") != "ada" || request.Form.Get("password") != "lovelace") {
			t.Errorf("unexpected credentials: %v", request.Form)
		}
		issued++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"access_token":"token-%d","refresh_token":"refresh-%d","expires_in":3600}`, issued, issued)
	}))
	defer tokenServer.Close()

	authentication := domain.Authentication{
		Type:      domain.AuthenticationOAuth2,
		GrantType: domain.OAuth2GrantPassword,
		TokenURL:  tokenServer.URL,
		ClientID:  "client-id",
		Username:  "{{user}}",
		Password:  "{{secret}}",
	}
	layers := []map[string]string{{"user": "ada", "secret": "lovelace"}}
	service := NewServiceWithDependencies(fakeSource{}, fakeBrowser{}, tokenServer.Client())
	provider := NewProvider(service)

	first, err := provider.AccessToken("/workspace", "c/r.yaml", "dev", authentication, layers)
	if err != nil || first != "token-1" {
		t.Fatalf("password grant: %q, %v", first, err)
	}
	again, err := provider.AccessToken("/workspace", "c/r.yaml", "dev", authentication, layers)
	if err != nil || again != "token-1" {
		t.Fatalf("a valid token must be reused: %q, %v", again, err)
	}

	// The API rejected the token: it is refreshed with the refresh token.
	provider.ExpireAccessToken("/workspace", "c/r.yaml", "dev")
	refreshed, err := provider.AccessToken("/workspace", "c/r.yaml", "dev", authentication, layers)
	if err != nil || refreshed != "token-2" {
		t.Fatalf("expected a refreshed token: %q, %v", refreshed, err)
	}
	if len(grants) != 2 || grants[0] != "password" || grants[1] != "refresh_token" {
		t.Fatalf("expected a password then a refresh exchange, got %v", grants)
	}
}

func TestPasswordGrantRequiresAUserName(t *testing.T) {
	_, err := resolveConfiguration(domain.Authentication{
		Type:      domain.AuthenticationOAuth2,
		GrantType: domain.OAuth2GrantPassword,
		TokenURL:  "https://identity.example.com/token",
		ClientID:  "client-id",
	}, nil)
	if err == nil {
		t.Fatal("expected a missing user name to be rejected")
	}
}

func TestTokenReturnsTheCachedTokens(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"access","id_token":"identity","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	service := NewServiceWithDependencies(
		fakeSource{request: clientCredentialsRequest(tokenServer.URL)},
		fakeBrowser{},
		tokenServer.Client(),
	)

	if _, err := service.Token("/workspace", "collection/request.yaml", "development"); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("expected ErrAuthorizationRequired before authorizing, got %v", err)
	}

	if _, err := service.Authorize("/workspace", "collection/request.yaml", "development"); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	details, err := service.Token("/workspace", "collection/request.yaml", "development")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if details.AccessToken != "access" || details.IDToken != "identity" || details.RefreshToken != "refresh" {
		t.Errorf("unexpected tokens: %+v", details)
	}
	if details.TokenType != "Bearer" || details.ExpiresAtMilliseconds == 0 {
		t.Errorf("unexpected token metadata: %+v", details)
	}
	if details.GrantType != domain.OAuth2GrantClientCredentials {
		t.Errorf("unexpected grant type: %q", details.GrantType)
	}
}

// ownerSource resolves every request to the collection that defines the
// authentication, as requests inheriting it do.
type ownerSource struct {
	fakeSource
}

func (source ownerSource) ResolveAuthentication(_ string, _ string) (domain.Authentication, string, error) {
	return *source.request.Authentication, "collections/keycloak", nil
}

func TestRequestsInheritingAnAuthenticationShareItsToken(t *testing.T) {
	var requests int
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"shared","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	request := clientCredentialsRequest(tokenServer.URL)
	service := NewServiceWithDependencies(ownerSource{fakeSource{request: request}}, fakeBrowser{}, tokenServer.Client())

	if _, err := service.Authorize("/workspace", "collections/keycloak", ""); err != nil {
		t.Fatalf("authorize the collection: %v", err)
	}

	token, err := NewProvider(service).AccessToken("/workspace", "collections/keycloak/users/list.yaml", "", *request.Authentication, nil)
	if err != nil || token != "shared" {
		t.Fatalf("expected the collection token, got %q, %v", token, err)
	}
	if requests != 1 {
		t.Errorf("expected the request to reuse the collection token, got %d token requests", requests)
	}

	status, err := service.Status("/workspace", "collections/keycloak/users/list.yaml", "")
	if err != nil || !status.HasToken {
		t.Errorf("expected the request to report the shared token, got %+v, %v", status, err)
	}
}

func TestTokenEndpointErrorNamesTheEndpointAndTheStatus(t *testing.T) {
	err := tokenEndpointError(
		"https://user:secret@id.example.com/realms/master/protocol/openid-connect/auth?debug=1",
		http.StatusMethodNotAllowed,
		[]byte(`{"error":"Unable to find target resource method"}`),
	)
	expected := "oauth2: token request failed: token endpoint https://id.example.com/realms/master/protocol/openid-connect/auth responded 405 Method Not Allowed: Unable to find target resource method"
	if !errors.Is(err, ErrTokenRequestFailed) || err.Error() != expected {
		t.Errorf("unexpected error:\n%v\nexpected:\n%s", err, expected)
	}

	err = tokenEndpointError("https://id.example.com/token", http.StatusNotFound, []byte("<html><body>Not found</body></html>"))
	if err.Error() != "oauth2: token request failed: token endpoint https://id.example.com/token responded 404 Not Found" {
		t.Errorf("an HTML body must not be quoted: %v", err)
	}

	err = tokenEndpointError("https://id.example.com/token", http.StatusBadGateway, []byte("upstream\n  unavailable"))
	if !strings.HasSuffix(err.Error(), "responded 502 Bad Gateway: upstream unavailable") {
		t.Errorf("a plain text body must be quoted on one line: %v", err)
	}
}

func TestTheEnvironmentCanSwitchTheAuthenticationToOAuth2(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		if request.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("expected the grant of the environment, got %q", request.Form.Get("grant_type"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"switched"}`))
	}))
	defer tokenServer.Close()

	request := clientCredentialsRequest(tokenServer.URL)
	request.Authentication.Type = domain.AuthenticationBearer
	request.Authentication.GrantType = domain.OAuth2GrantPassword
	source := fakeSource{
		request: request,
		layers:  []map[string]string{{"authType": "oauth2", "grantType": "client_credentials"}},
	}
	service := NewServiceWithDependencies(source, fakeBrowser{}, tokenServer.Client())

	status, err := service.Authorize("/workspace", "collections/keycloak", "prod")
	if err != nil || !status.HasToken || status.GrantType != domain.OAuth2GrantClientCredentials {
		t.Fatalf("expected the environment to choose OAuth 2 client credentials, got %+v, %v", status, err)
	}
}
