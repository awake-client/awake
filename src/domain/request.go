// Package domain holds the entities shared by several features. It contains no
// input or output: persistence lives in features and file system access lives
// in shared packages.
package domain

import (
	"errors"
	"net/url"
	"regexp"
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
	Name       string      `yaml:"name"`
	Method     string      `yaml:"method"`
	URL        string      `yaml:"url"`
	Protocol   Protocol    `yaml:"protocol,omitempty"`
	Parameters []Parameter `yaml:"parameters,omitempty"`
	// PathParameters give the values of the :name segments of the URL.
	PathParameters []PathParameter `yaml:"pathParameters,omitempty"`
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
	// Fields holds the fields of a form body, form-urlencoded or multipart,
	// in place of Content. Each one is encoded after its variables are
	// resolved.
	Fields []Parameter `yaml:"fields,omitempty"`
}

// IsForm reports whether the body is made of fields rather than text.
func (body Body) IsForm() bool {
	return body.Type == BodyFormURLEncoded || body.Type == BodyMultipart
}

// NormalizeForm moves a form written as text, by earlier versions or
// imports, into fields: an encoded query such as a=1&b=2, or one field per
// line written name: value or name=value, a leading ~ disabling it.
func (body *Body) NormalizeForm() {
	if !body.IsForm() || len(body.Fields) > 0 || strings.TrimSpace(body.Content) == "" {
		return
	}
	body.Fields = parseFormContent(body.Content)
	body.Content = ""
}

func parseFormContent(content string) []Parameter {
	trimmed := strings.TrimSpace(content)
	fields := make([]Parameter, 0)

	if !strings.Contains(trimmed, "\n") && !strings.Contains(trimmed, ": ") {
		for _, pair := range strings.Split(trimmed, "&") {
			if pair == "" {
				continue
			}
			name, value, _ := strings.Cut(pair, "=")
			fields = append(fields, Parameter{Name: unescapeForm(name), Value: unescapeForm(value), Enabled: true})
		}
		return fields
	}

	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		enabled := !strings.HasPrefix(line, "~")
		line = strings.TrimSpace(strings.TrimPrefix(line, "~"))

		separator := strings.IndexAny(line, ":=")
		if separator <= 0 {
			continue
		}
		fields = append(fields, Parameter{
			Name:    strings.TrimSpace(line[:separator]),
			Value:   strings.TrimSpace(line[separator+1:]),
			Enabled: enabled,
		})
	}
	return fields
}

func unescapeForm(value string) string {
	unescaped, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return unescaped
}

// PathParameter is the value of a :name segment of a request URL.
type PathParameter struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// URLPlaceholder is a :name in a URL, filled from the path parameters of
// the request: a path segment written /:name, or a query value written
// name=:name. The colon must follow a slash or an equals sign, so a scheme,
// a port or a time such as 12:30 is never read as one.
type URLPlaceholder struct {
	Name string
	// InQuery tells a query value, escaped for a query, from a path segment.
	InQuery bool
	// Start and End delimit the name and its colon in the URL.
	Start int
	End   int
}

var (
	pathPlaceholderPattern  = regexp.MustCompile(`/(:[A-Za-z_][A-Za-z0-9_]*)`)
	queryPlaceholderPattern = regexp.MustCompile(`=(:[A-Za-z_][A-Za-z0-9_]*)`)
)

// URLPlaceholders returns the placeholders of a URL in order. A query
// placeholder must be a whole value, ending the URL or followed by & or #.
func URLPlaceholders(rawURL string) []URLPlaceholder {
	queryStart := strings.Index(rawURL, "?")
	pathPart := rawURL
	if queryStart >= 0 {
		pathPart = rawURL[:queryStart]
	}

	placeholders := make([]URLPlaceholder, 0)
	for _, match := range pathPlaceholderPattern.FindAllStringSubmatchIndex(pathPart, -1) {
		placeholders = append(placeholders, URLPlaceholder{Name: rawURL[match[2]+1 : match[3]], Start: match[2], End: match[3]})
	}
	if queryStart < 0 {
		return placeholders
	}

	query := rawURL[queryStart:]
	for _, match := range queryPlaceholderPattern.FindAllStringSubmatchIndex(query, -1) {
		if match[3] < len(query) && query[match[3]] != '&' && query[match[3]] != '#' {
			continue
		}
		start, end := queryStart+match[2], queryStart+match[3]
		placeholders = append(placeholders, URLPlaceholder{Name: rawURL[start+1 : end], InQuery: true, Start: start, End: end})
	}
	return placeholders
}

// PathParameterNames returns the names of the placeholders of a URL, in
// order and once each.
func PathParameterNames(rawURL string) []string {
	names := make([]string, 0)
	seen := map[string]bool{}
	for _, placeholder := range URLPlaceholders(rawURL) {
		if !seen[placeholder.Name] {
			seen[placeholder.Name] = true
			names = append(names, placeholder.Name)
		}
	}
	return names
}

// ReplacePathParameters replaces each placeholder of a URL, colon included,
// with the value resolve returns for it.
func ReplacePathParameters(rawURL string, resolve func(placeholder URLPlaceholder) (string, error)) (string, error) {
	var builder strings.Builder
	cursor := 0
	for _, placeholder := range URLPlaceholders(rawURL) {
		value, err := resolve(placeholder)
		if err != nil {
			return rawURL, err
		}
		builder.WriteString(rawURL[cursor:placeholder.Start])
		builder.WriteString(value)
		cursor = placeholder.End
	}
	builder.WriteString(rawURL[cursor:])
	return builder.String(), nil
}

// RequestOverride holds the parts of a request replaced on this machine only.
// It lives next to the request in <name>.local.yaml, which git ignores. A nil
// section keeps the shared one; a set section, even empty, replaces it.
type RequestOverride struct {
	Parameters     *[]Parameter     `yaml:"parameters,omitempty"`
	PathParameters *[]PathParameter `yaml:"pathParameters,omitempty"`
	Body           *Body            `yaml:"body,omitempty"`
}

// IsEmpty reports whether the override replaces nothing.
func (override RequestOverride) IsEmpty() bool {
	return override.Parameters == nil && override.PathParameters == nil && override.Body == nil
}

// ApplyTo returns request with the overridden sections replaced.
func (override RequestOverride) ApplyTo(request Request) Request {
	if override.Parameters != nil {
		request.Parameters = append([]Parameter(nil), (*override.Parameters)...)
	}
	if override.PathParameters != nil {
		request.PathParameters = append([]PathParameter(nil), (*override.PathParameters)...)
	}
	if override.Body != nil {
		body := *override.Body
		request.Body = &body
	}
	return request
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
	// EnvironmentName is the display name of Environment, for the interface.
	EnvironmentName string
	Name            string
	Value           string
	Removed         bool
}

// EnvironmentResolver gives scripts access to environments other than the
// selected one.
type EnvironmentResolver interface {
	// ResolveEnvironment finds an environment by file name or display name.
	ResolveEnvironment(name string) (ScriptEnvironment, error)
}

// ScriptEnvironment is an environment opened by a script, with the variables
// a request would see in it.
type ScriptEnvironment struct {
	FileName  string
	Name      string
	Variables map[string]string
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
	request.Body.NormalizeForm()
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
