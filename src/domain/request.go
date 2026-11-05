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
}

// Body holds the request body.
type Body struct {
	Type    BodyType `yaml:"type"`
	Content string   `yaml:"content,omitempty"`
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

	if request.Authentication != nil && !isSupportedAuthentication(request.Authentication.Type) {
		return ErrInvalidRequest
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

func isSupportedBodyType(bodyType BodyType) bool {
	switch bodyType {
	case BodyNone, BodyJSON, BodyText, BodyXML, BodyFormURLEncoded, BodyMultipart, BodyBinary:
		return true
	default:
		return false
	}
}
