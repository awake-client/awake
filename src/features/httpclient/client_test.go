package httpclient

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/shared/variables"
)

type fakeSource struct {
	request domain.Request
	layers  []map[string]string
	err     error
}

func (source fakeSource) LoadRequest(_ string, _ string) (domain.Request, error) {
	return source.request, source.err
}

func (source fakeSource) LoadVariableLayers(_ string, _ string) ([]map[string]string, error) {
	return source.layers, nil
}

func TestSendInterpolatesRequestAndAuthentication(t *testing.T) {
	var received struct {
		method        string
		query         string
		authorization string
		customHeader  string
		contentType   string
		body          string
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		received.method = request.Method
		received.query = request.URL.RawQuery
		received.authorization = request.Header.Get("Authorization")
		received.customHeader = request.Header.Get("X-Workspace")
		received.contentType = request.Header.Get("Content-Type")
		received.body = string(body)

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "Create user",
			Method: domain.MethodPost,
			URL:    "{{baseUrl}}/users",
			Parameters: []domain.Parameter{
				{Name: "page", Value: "{{page}}", Enabled: true},
				{Name: "ignored", Value: "yes", Enabled: false},
			},
			Headers: []domain.Header{
				{Name: "X-Workspace", Value: "{{workspaceName}}", Enabled: true},
			},
			Authentication: &domain.Authentication{
				Type:  domain.AuthenticationBearer,
				Token: "{{token}}",
			},
			Body: &domain.Body{
				Type:    domain.BodyJSON,
				Content: `{"name":"{{userName}}"}`,
			},
		},
		layers: []map[string]string{
			{
				"baseUrl":       server.URL,
				"page":          "2",
				"workspaceName": "platform",
				"token":         "secret-token",
				"userName":      "Ada",
			},
		},
	})

	response, err := service.Send(t.TempDir(), "collections/platform/users/create.yaml", "development")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	if received.method != http.MethodPost {
		t.Errorf("expected POST, got %s", received.method)
	}
	if received.query != "page=2" {
		t.Errorf("unexpected query %q", received.query)
	}
	if received.authorization != "Bearer secret-token" {
		t.Errorf("unexpected authorization %q", received.authorization)
	}
	if received.customHeader != "platform" {
		t.Errorf("unexpected custom header %q", received.customHeader)
	}
	if received.contentType != "application/json" {
		t.Errorf("unexpected content type %q", received.contentType)
	}
	if received.body != `{"name":"Ada"}` {
		t.Errorf("unexpected body %q", received.body)
	}

	if response.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", response.StatusCode)
	}
	if response.Encoding != "utf-8" {
		t.Errorf("expected utf-8 encoding, got %q", response.Encoding)
	}
	if !strings.Contains(response.Preview, `"ok":true`) {
		t.Errorf("unexpected preview %q", response.Preview)
	}
}

func TestSendUsesBasicAuthentication(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "Basic",
			Method: domain.MethodGet,
			URL:    server.URL,
			Authentication: &domain.Authentication{
				Type:     domain.AuthenticationBasic,
				Username: "{{user}}",
				Password: "{{password}}",
			},
		},
		layers: []map[string]string{{"user": "ada", "password": "lovelace"}},
	})

	response, err := service.Send(t.TempDir(), "collections/basic.yaml", "")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("ada:lovelace"))
	if authorization != expected {
		t.Errorf("unexpected authorization %q", authorization)
	}
}

func TestSendStoresLargeBodyAndReadsItInChunks(t *testing.T) {
	content := strings.Repeat("a", PreviewSize+250<<10)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte(content))
	}))
	defer server.Close()

	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "Large",
			Method: domain.MethodGet,
			URL:    server.URL,
		},
	})

	response, err := service.Send(t.TempDir(), "collections/large.yaml", "")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	if response.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), response.Size)
	}
	if len(response.Preview) != PreviewSize {
		t.Errorf("expected preview of %d bytes, got %d", PreviewSize, len(response.Preview))
	}

	chunk, err := service.ReadChunk(response.ID, PreviewSize, 1024)
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	if len(chunk.Content) != 1024 {
		t.Errorf("expected 1024 bytes, got %d", len(chunk.Content))
	}
	if chunk.NextOffset != PreviewSize+1024 {
		t.Errorf("unexpected next offset %d", chunk.NextOffset)
	}
	if !chunk.HasMore {
		t.Error("expected more chunks")
	}

	lastChunk, err := service.ReadChunk(response.ID, response.Size-10, 1024)
	if err != nil {
		t.Fatalf("read last chunk: %v", err)
	}
	if lastChunk.HasMore {
		t.Error("expected the last chunk")
	}
	if lastChunk.NextOffset != response.Size {
		t.Errorf("unexpected final offset %d", lastChunk.NextOffset)
	}
}

func TestSendEncodesBinaryResponseAsBase64(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = writer.Write([]byte{0x00, 0x01, 0xff, 0xfe})
	}))
	defer server.Close()

	service := newTestService(t, fakeSource{
		request: domain.Request{Name: "Binary", Method: domain.MethodGet, URL: server.URL},
	})

	response, err := service.Send(t.TempDir(), "collections/binary.yaml", "")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	if response.Encoding != "base64" {
		t.Fatalf("expected base64 encoding, got %q", response.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(response.Preview)
	if err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if len(decoded) != 4 || decoded[0] != 0x00 || decoded[3] != 0xfe {
		t.Errorf("unexpected binary preview %v", decoded)
	}
}

func TestSendReportsMissingVariables(t *testing.T) {
	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "Missing",
			Method: domain.MethodGet,
			URL:    "https://example.com/{{missingVariable}}",
		},
	})

	_, err := service.Send(t.TempDir(), "collections/missing.yaml", "")
	if err == nil {
		t.Fatal("expected an error")
	}

	var missingError *variables.MissingVariablesError
	if !errors.As(err, &missingError) {
		t.Fatalf("expected MissingVariablesError, got %v", err)
	}
}

func TestSendRejectsUnsupportedBodyTypes(t *testing.T) {
	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "Multipart",
			Method: domain.MethodPost,
			URL:    "https://example.com",
			Body:   &domain.Body{Type: domain.BodyMultipart},
		},
	})

	_, err := service.Send(t.TempDir(), "collections/multipart.yaml", "")
	if !errors.Is(err, ErrUnsupportedBody) {
		t.Fatalf("expected ErrUnsupportedBody, got %v", err)
	}
}

func TestCloseRemovesStoredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("content"))
	}))
	defer server.Close()

	service := newTestService(t, fakeSource{
		request: domain.Request{Name: "Close", Method: domain.MethodGet, URL: server.URL},
	})

	response, err := service.Send(t.TempDir(), "collections/close.yaml", "")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}

	if err := service.Close(response.ID); err != nil {
		t.Fatalf("close response: %v", err)
	}
	if _, err := service.ReadChunk(response.ID, 0, 10); !errors.Is(err, ErrResponseNotFound) {
		t.Fatalf("expected ErrResponseNotFound, got %v", err)
	}
}

func newTestService(t *testing.T, source Source) *Service {
	t.Helper()

	responseStore, err := newStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("create response store: %v", err)
	}

	return newServiceWithStore(source, responseStore)
}
