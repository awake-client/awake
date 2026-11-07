// Package domain holds the entities shared by several features. It contains no
// input or output: persistence lives in features and file system access lives
// in shared packages.
package domain

import (
	"errors"
	"strings"
)

// ErrInvalidRequest is returned when a request file is malformed.
var ErrInvalidRequest = errors.New("domain: invalid request")

// HTTP methods supported by the request editor.
const (
	MethodGet     = "GET"
	MethodPost    = "POST"
	MethodPut     = "PUT"
	MethodPatch   = "PATCH"
	MethodDelete  = "DELETE"
	MethodHead    = "HEAD"
	MethodOptions = "OPTIONS"
)

// Protocol describes how a request exchanges data with its server.
type Protocol string

const (
	// ProtocolHTTP is a single request and response exchange.
	ProtocolHTTP Protocol = "http"
	// ProtocolSSE keeps an HTTP connection open and reads server-sent events.
	ProtocolSSE Protocol = "sse"
	// ProtocolWebSocket keeps a bidirectional WebSocket connection open.
	ProtocolWebSocket Protocol = "websocket"
)

// Body types supported by the request editor.
type BodyType string

const (
	BodyNone           BodyType = "none"
	BodyJSON           BodyType = "json"
	BodyText           BodyType = "text"
	BodyXML            BodyType = "xml"
	BodyFormURLEncoded BodyType = "form-urlencoded"
	BodyMultipart      BodyType = "multipart"
	BodyBinary         BodyType = "binary"
)

// AuthenticationType describes how a request authenticates.
type AuthenticationType string

const (
	AuthenticationInherit AuthenticationType = "inherit"
	AuthenticationNone    AuthenticationType = "none"
	AuthenticationBasic   AuthenticationType = "basic"
	AuthenticationBearer  AuthenticationType = "bearer"
	AuthenticationAPIKey  AuthenticationType = "apiKey"
	AuthenticationOAuth2  AuthenticationType = "oauth2"
)

// OAuth2GrantType describes how an OAuth 2 client obtains a token.
type OAuth2GrantType string

const (
	// OAuth2GrantAuthorizationCode opens a browser and receives a code on a
	// loopback redirect with PKCE.
	OAuth2GrantAuthorizationCode OAuth2GrantType = "authorizationCode"
	// OAuth2GrantClientCredentials exchanges the client credentials directly.
	OAuth2GrantClientCredentials OAuth2GrantType = "clientCredentials"
	// OAuth2GrantPassword exchanges a user name and password, for services
	// that still offer the resource owner password credentials grant.
	OAuth2GrantPassword OAuth2GrantType = "password"
)

// Request mirrors a request YAML file.
type Request struct {
	Name           string          `yaml:"name"`
	Method         string          `yaml:"method"`
	URL            string          `yaml:"url"`
	Protocol       Protocol        `yaml:"protocol,omitempty"`
	Parameters     []Parameter     `yaml:"parameters,omitempty"`
	Headers        []Header        `yaml:"headers,omitempty"`
	Authentication *Authentication `yaml:"authentication,omitempty"`
	Body           *Body           `yaml:"body,omitempty"`
	Scripts        *Scripts        `yaml:"scripts,omitempty"`
	Documentation  string          `yaml:"documentation,omitempty"`
}

// Parameter is a query parameter.
type Parameter struct {
	Name    string `yaml:"name"`
	Value   string `yaml:"value"`
	Enabled bool   `yaml:"enabled"`
}

// Header is a request header.
type Header struct {
	Name    string `yaml:"name"`
	Value   string `yaml:"value"`
	Enabled bool   `yaml:"enabled"`
}

// Authentication holds the credentials configuration of a request. The values
// may reference variables; secrets are never stored here.
type Authentication struct {
	Type     AuthenticationType `yaml:"type"`
	Username string             `yaml:"username,omitempty"`
	Password string             `yaml:"password,omitempty"`
	Token    string             `yaml:"token,omitempty"`
	Key      string             `yaml:"key,omitempty"`
	Value    string             `yaml:"value,omitempty"`
	Location string             `yaml:"location,omitempty"`

	// OAuth 2 settings. ClientSecret may reference a secret variable; the
	// value itself is never written to the workspace.
	GrantType        OAuth2GrantType `yaml:"grantType,omitempty"`
	AuthorizationURL string          `yaml:"authorizationUrl,omitempty"`
	TokenURL         string          `yaml:"tokenUrl,omitempty"`
	ClientID         string          `yaml:"clientId,omitempty"`
	ClientSecret     string          `yaml:"clientSecret,omitempty"`
	Scopes           []string        `yaml:"scopes,omitempty"`
	RedirectURL      string          `yaml:"redirectUrl,omitempty"`
	Audience         string          `yaml:"audience,omitempty"`
}

// Body holds the request body.
type Body struct {
	Type    BodyType `yaml:"type"`
	Content string   `yaml:"content,omitempty"`
}

// Scripts holds the JavaScript hooks of a request. They run inside the
// application with access to the request, the response and the resolved
// variables.
type Scripts struct {
	PreRequest   string `yaml:"preRequest,omitempty"`
	PostResponse string `yaml:"postResponse,omitempty"`
}

// ScriptTest is one assertion reported by a request script.
type ScriptTest struct {
	Name   string
	Passed bool
	Error  string
}

// EnvironmentChange is a variable a script stores in, or removes from, the
// selected environment so later requests see it.
type EnvironmentChange struct {
	// Environment is the file name of the environment to change, or empty
	// for the selected environment.
	Environment string
	Name        string
	Value       string
	Removed     bool
}

// EnvironmentResolver gives scripts access to environments other than the
// selected one.
type EnvironmentResolver interface {
	// ResolveEnvironment finds an environment by file name or display name
	// and returns its file name with the variables a request would see.
	ResolveEnvironment(name string) (fileName string, variables map[string]string, err error)
}

// ScriptOutcome is the result of a pre-request or post-response script.
type ScriptOutcome struct {
	Logs  []string
	Tests []ScriptTest
	Error string
	// EnvironmentChanges lists, in order, the variables the script stored in
	// the selected environment.
	EnvironmentChanges []EnvironmentChange
	// EnvironmentError explains why the environment changes were not saved.
	EnvironmentError string
}

// ScriptResponse is the response view exposed to post-response scripts. Body
// contains the preview (or the base64 preview for binary responses).
type ScriptResponse struct {
	StatusCode           int
	Status               string
	Headers              map[string][]string
	Body                 string
	Encoding             string
	DurationMilliseconds int64
}

// ApplyDefaults fills optional sections and normalizes the method.
func (request *Request) ApplyDefaults() {
	request.Method = strings.ToUpper(strings.TrimSpace(request.Method))

	if request.Protocol == "" {
		request.Protocol = ProtocolHTTP
	}
	if request.Authentication == nil {
		request.Authentication = &Authentication{Type: AuthenticationInherit}
	}
	if request.Authentication.Type == AuthenticationOAuth2 &&
		request.Authentication.GrantType == "" &&
		request.Authentication.Token == "" {
		request.Authentication.GrantType = OAuth2GrantAuthorizationCode
	}
	if request.Body == nil {
		request.Body = &Body{Type: BodyNone}
	}
}

// Validate checks the required fields and the supported enumerations.
func (request Request) Validate() error {
	if strings.TrimSpace(request.Name) == "" {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(request.URL) == "" {
		return ErrInvalidRequest
	}
	if !isSupportedMethod(request.Method) {
		return ErrInvalidRequest
	}
	if !isSupportedProtocol(request.Protocol) {
		return ErrInvalidRequest
	}

	for _, parameter := range request.Parameters {
		if strings.TrimSpace(parameter.Name) == "" {
			return ErrInvalidRequest
		}
	}
	for _, header := range request.Headers {
		if strings.TrimSpace(header.Name) == "" {
			return ErrInvalidRequest
		}
	}

	if request.Authentication != nil {
		if err := validateAuthentication(*request.Authentication); err != nil {
			return err
		}
	}
	if request.Body != nil && !isSupportedBodyType(request.Body.Type) {
		return ErrInvalidRequest
	}

	return nil
}

func isSupportedMethod(method string) bool {
	switch method {
	case MethodGet, MethodPost, MethodPut, MethodPatch, MethodDelete, MethodHead, MethodOptions:
		return true
	default:
		return false
	}
}

func isSupportedProtocol(protocol Protocol) bool {
	switch protocol {
	case "", ProtocolHTTP, ProtocolSSE, ProtocolWebSocket:
		return true
	default:
		return false
	}
}

func isSupportedAuthentication(authenticationType AuthenticationType) bool {
	switch authenticationType {
	case AuthenticationInherit, AuthenticationNone, AuthenticationBasic,
		AuthenticationBearer, AuthenticationAPIKey, AuthenticationOAuth2:
		return true
	default:
		return false
	}
}

func validateAuthentication(authentication Authentication) error {
	if !isSupportedAuthentication(authentication.Type) {
		return ErrInvalidRequest
	}
	if authentication.Type != AuthenticationOAuth2 {
		return nil
	}

	// A bare access token remains valid for requests written before the
	// OAuth 2 flows existed.
	if authentication.GrantType == "" && authentication.Token != "" {
		return nil
	}
	if !isSupportedOAuth2GrantType(authentication.GrantType) {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(authentication.TokenURL) == "" ||
		strings.TrimSpace(authentication.ClientID) == "" {
		return ErrInvalidRequest
	}
	if authentication.GrantType == OAuth2GrantAuthorizationCode &&
		strings.TrimSpace(authentication.AuthorizationURL) == "" {
		return ErrInvalidRequest
	}
	if authentication.GrantType == OAuth2GrantPassword &&
		strings.TrimSpace(authentication.Username) == "" {
		return ErrInvalidRequest
	}

	return nil
}

func isSupportedOAuth2GrantType(grantType OAuth2GrantType) bool {
	switch grantType {
	case OAuth2GrantAuthorizationCode, OAuth2GrantClientCredentials, OAuth2GrantPassword:
		return true
	default:
		return false
	}
}

func isSupportedBodyType(bodyType BodyType) bool {
	switch bodyType {
	case BodyNone, BodyJSON, BodyText, BodyXML, BodyFormURLEncoded, BodyMultipart, BodyBinary:
		return true
	default:
		return false
	}
}
