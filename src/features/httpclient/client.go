package httpclient

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/shared/variables"
)

const userAgent = "Awake"

// Source supplies the request and the variable layers to execute. The
// application layer implements it so this feature does not depend on the
// internals of the workspace, collections or environments features.
type Source interface {
	LoadRequest(workspaceRoot string, relativePath string) (domain.Request, error)
	LoadVariableLayers(workspaceRoot string, environmentFileName string) ([]map[string]string, error)
}

// HistoryRecorder records executed requests. The application layer supplies
// the implementation so this feature does not depend on the history feature.
type HistoryRecorder interface {
	Record(workspaceRoot string, entry domain.HistoryEntry) error
}

// OAuth2TokenProvider supplies a bearer token for an OAuth 2 request. The
// application layer supplies the implementation so this feature does not
// depend on the OAuth 2 feature.
type OAuth2TokenProvider interface {
	AccessToken(
		workspaceRoot string,
		requestPath string,
		environmentFileName string,
		authentication domain.Authentication,
		layers []map[string]string,
	) (string, error)
}

// Service exposes request execution to the application layer.
type Service struct {
	source       Source
	client       *http.Client
	streamClient *http.Client
	streams      *streamRegistry
	store        *store
	recorder     HistoryRecorder
	oauth2       OAuth2TokenProvider
}

// NewService returns an HTTP client service backed by source.
func NewService(source Source, recorder HistoryRecorder) (*Service, error) {
	responseStore, err := newStore()
	if err != nil {
		return nil, err
	}

	return newServiceWithStore(source, responseStore, recorder), nil
}

func newServiceWithStore(source Source, responseStore *store, recorder HistoryRecorder) *Service {
	return &Service{
		source:       source,
		client:       &http.Client{Timeout: DefaultTimeout},
		streamClient: newStreamClient(),
		streams:      newStreamRegistry(),
		store:        responseStore,
		recorder:     recorder,
	}
}

// SetOAuth2Provider connects the OAuth 2 flows to request execution. It is a
// package function rather than a service method, so the provider never crosses
// the Wails bridge.
func SetOAuth2Provider(service *Service, provider OAuth2TokenProvider) {
	service.oauth2 = provider
}

// Send executes a request and streams its body to the response store.
func (service *Service) Send(workspaceRoot string, requestPath string, environmentFileName string) (Response, error) {
	request, err := service.source.LoadRequest(workspaceRoot, requestPath)
	if err != nil {
		return Response{}, err
	}

	layers, err := service.source.LoadVariableLayers(workspaceRoot, environmentFileName)
	if err != nil {
		return Response{}, err
	}

	oauth2Token, err := service.oauth2Token(workspaceRoot, requestPath, environmentFileName, request, layers)
	if err != nil {
		return Response{}, err
	}

	httpRequest, err := buildHTTPRequest(request, layers, oauth2Token)
	if err != nil {
		return Response{}, err
	}

	startedAt := time.Now()
	httpResponse, err := service.client.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("send request: %w", err)
	}
	defer httpResponse.Body.Close()

	response, err := service.captureResponse(httpResponse, time.Since(startedAt))
	if err != nil {
		return Response{}, err
	}

	response.HistoryWarning = service.recordHistory(workspaceRoot, requestPath, environmentFileName, request, response)

	return response, nil
}

func (service *Service) oauth2Token(
	workspaceRoot string,
	requestPath string,
	environmentFileName string,
	request domain.Request,
	layers []map[string]string,
) (string, error) {
	authentication := resolvedAuthentication(request.Authentication)
	if authentication.Type != domain.AuthenticationOAuth2 || authentication.GrantType == "" {
		// A static OAuth 2 token or another authentication type needs no
		// exchange.
		return "", nil
	}
	if service.oauth2 == nil {
		return "", ErrOAuth2AuthorizationRequired
	}

	return service.oauth2.AccessToken(
		workspaceRoot,
		requestPath,
		environmentFileName,
		authentication,
		layers,
	)
}

func (service *Service) recordHistory(workspaceRoot string, requestPath string, environmentFileName string, request domain.Request, response Response) string {
	if service.recorder == nil {
		return ""
	}

	err := service.recorder.Record(workspaceRoot, domain.HistoryEntry{
		RequestPath:          requestPath,
		RequestName:          request.Name,
		Method:               request.Method,
		URL:                  request.URL,
		StatusCode:           response.StatusCode,
		DurationMilliseconds: response.DurationMilliseconds,
		SizeBytes:            response.Size,
		Environment:          environmentFileName,
	})
	if err != nil {
		return err.Error()
	}

	return ""
}

// ReadChunk returns a slice of a stored response body.
func (service *Service) ReadChunk(identifier string, offset int64, length int) (Chunk, error) {
	return service.store.readChunk(identifier, offset, length)
}

// Close releases a stored response and its temporary file.
func (service *Service) Close(identifier string) error {
	return service.store.delete(identifier)
}

func (service *Service) captureResponse(httpResponse *http.Response, duration time.Duration) (Response, error) {
	temporaryFile, err := service.store.temporaryFile()
	if err != nil {
		return Response{}, fmt.Errorf("store response: %w", err)
	}
	temporaryPath := temporaryFile.Name()

	keepFile := false
	defer func() {
		temporaryFile.Close()
		if !keepFile {
			_ = os.Remove(temporaryPath)
		}
	}()

	limitedReader := io.LimitReader(httpResponse.Body, MaximumBodySize+1)
	written, err := io.Copy(temporaryFile, limitedReader)
	if err != nil {
		return Response{}, fmt.Errorf("read response body: %w", err)
	}

	truncated := written > MaximumBodySize
	if truncated {
		written = MaximumBodySize
		if err := temporaryFile.Truncate(written); err != nil {
			return Response{}, fmt.Errorf("truncate response body: %w", err)
		}
	}

	if err := temporaryFile.Close(); err != nil {
		return Response{}, fmt.Errorf("close response body: %w", err)
	}

	preview, err := readPreview(temporaryPath, written)
	if err != nil {
		return Response{}, err
	}

	contentType := httpResponse.Header.Get("Content-Type")
	textual := isTextual(contentType, preview)
	encoding := "utf-8"
	if !textual || (len(preview) > 0 && !utf8.Valid(preview)) {
		encoding = "base64"
	}

	identifier, err := service.store.register(temporaryPath, contentType, encoding, written)
	if err != nil {
		return Response{}, err
	}
	keepFile = true

	return Response{
		ID:                   identifier,
		StatusCode:           httpResponse.StatusCode,
		Status:               httpResponse.Status,
		Headers:              httpResponse.Header,
		DurationMilliseconds: duration.Milliseconds(),
		Size:                 written,
		ContentType:          contentType,
		Preview:              encodeBytes(preview, encoding),
		Encoding:             encoding,
		Truncated:            truncated,
	}, nil
}

func readPreview(path string, size int64) ([]byte, error) {
	length := size
	if length > PreviewSize {
		length = PreviewSize
	}
	if length == 0 {
		return []byte{}, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open response body: %w", err)
	}
	defer file.Close()

	buffer := make([]byte, length)
	read, err := io.ReadFull(file, buffer)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, fmt.Errorf("read response preview: %w", err)
	}

	return buffer[:read], nil
}

func buildHTTPRequest(request domain.Request, layers []map[string]string, oauth2Token string) (*http.Request, error) {
	return buildHTTPRequestWithContext(context.Background(), request, layers, oauth2Token)
}

func buildHTTPRequestWithContext(parent context.Context, request domain.Request, layers []map[string]string, oauth2Token string) (*http.Request, error) {
	rawURL, err := interpolate(request.URL, layers)
	if err != nil {
		return nil, err
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse request URL: %w", err)
	}

	query := parsedURL.Query()

	for _, parameter := range request.Parameters {
		if !parameter.Enabled {
			continue
		}

		name, err := interpolate(parameter.Name, layers)
		if err != nil {
			return nil, err
		}
		value, err := interpolate(parameter.Value, layers)
		if err != nil {
			return nil, err
		}

		query.Add(name, value)
	}

	authentication := resolvedAuthentication(request.Authentication)

	bodyReader, contentType, err := buildBody(request.Body, layers)
	if err != nil {
		return nil, err
	}

	if authentication.Type == domain.AuthenticationAPIKey &&
		authentication.Location == "query" {
		key, err := interpolate(authentication.Key, layers)
		if err != nil {
			return nil, err
		}
		value, err := interpolate(authentication.Value, layers)
		if err != nil {
			return nil, err
		}
		query.Set(key, value)
	}

	parsedURL.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(parent, request.Method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for _, header := range request.Headers {
		if !header.Enabled {
			continue
		}

		name, err := interpolate(header.Name, layers)
		if err != nil {
			return nil, err
		}
		value, err := interpolate(header.Value, layers)
		if err != nil {
			return nil, err
		}

		httpRequest.Header.Set(name, value)
	}

	if httpRequest.Header.Get("User-Agent") == "" {
		httpRequest.Header.Set("User-Agent", userAgent)
	}
	if contentType != "" && httpRequest.Header.Get("Content-Type") == "" {
		httpRequest.Header.Set("Content-Type", contentType)
	}

	if err := applyAuthentication(httpRequest, authentication, layers, oauth2Token); err != nil {
		return nil, err
	}

	return httpRequest, nil
}

func resolvedAuthentication(authentication *domain.Authentication) domain.Authentication {
	if authentication == nil {
		return domain.Authentication{Type: domain.AuthenticationNone}
	}
	if authentication.Type == domain.AuthenticationInherit {
		// Collection-level authentication is applied when the collections
		// feature exposes inherited credentials.
		return domain.Authentication{Type: domain.AuthenticationNone}
	}
	return *authentication
}

func applyAuthentication(httpRequest *http.Request, authentication domain.Authentication, layers []map[string]string, oauth2Token string) error {
	switch authentication.Type {
	case domain.AuthenticationBasic:
		username, err := interpolate(authentication.Username, layers)
		if err != nil {
			return err
		}
		password, err := interpolate(authentication.Password, layers)
		if err != nil {
			return err
		}
		httpRequest.SetBasicAuth(username, password)

	case domain.AuthenticationBearer:
		token, err := interpolate(authentication.Token, layers)
		if err != nil {
			return err
		}
		httpRequest.Header.Set("Authorization", "Bearer "+token)

	case domain.AuthenticationOAuth2:
		// Requests written before the flows existed carry a static token.
		if authentication.GrantType == "" {
			token, err := interpolate(authentication.Token, layers)
			if err != nil {
				return err
			}
			httpRequest.Header.Set("Authorization", "Bearer "+token)
			return nil
		}
		if oauth2Token == "" {
			return ErrOAuth2AuthorizationRequired
		}
		httpRequest.Header.Set("Authorization", "Bearer "+oauth2Token)

	case domain.AuthenticationAPIKey:
		if authentication.Location == "query" {
			// Already added to the query string while building the URL.
			return nil
		}

		key, err := interpolate(authentication.Key, layers)
		if err != nil {
			return err
		}
		value, err := interpolate(authentication.Value, layers)
		if err != nil {
			return err
		}
		httpRequest.Header.Set(key, value)
	}

	return nil
}

func buildBody(body *domain.Body, layers []map[string]string) (io.Reader, string, error) {
	if body == nil || body.Type == domain.BodyNone {
		return nil, "", nil
	}

	if body.Type == domain.BodyMultipart || body.Type == domain.BodyBinary {
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedBody, body.Type)
	}

	content, err := interpolate(body.Content, layers)
	if err != nil {
		return nil, "", err
	}

	return strings.NewReader(content), defaultContentType(body.Type), nil
}

func defaultContentType(bodyType domain.BodyType) string {
	switch bodyType {
	case domain.BodyJSON:
		return "application/json"
	case domain.BodyText:
		return "text/plain"
	case domain.BodyXML:
		return "application/xml"
	case domain.BodyFormURLEncoded:
		return "application/x-www-form-urlencoded"
	default:
		return ""
	}
}

func interpolate(value string, layers []map[string]string) (string, error) {
	return variables.Interpolate(value, layers...)
}

func isTextual(contentType string, preview []byte) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && mediaType != "" {
		if strings.HasPrefix(mediaType, "text/") {
			return true
		}
		switch mediaType {
		case "application/json", "application/xml", "application/javascript",
			"application/x-www-form-urlencoded", "application/yaml",
			"application/x-yaml", "application/graphql":
			return true
		}
		if strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml") {
			return true
		}
		return false
	}

	if len(preview) == 0 {
		return true
	}

	return strings.HasPrefix(http.DetectContentType(preview), "text/")
}
