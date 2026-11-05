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

type fakeRecorder struct {
	entries []domain.HistoryEntry
	err     error
}

func (recorder *fakeRecorder) Record(_ string, entry domain.HistoryEntry) error {
	recorder.entries = append(recorder.entries, entry)
	return recorder.err
}

func TestSendRecordsHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte("hello"))
	}))
	defer server.Close()

	responseStore, err := newStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("create response store: %v", err)
	}

	recorder := &fakeRecorder{}
	service := newServiceWithStore(fakeSource{
		request: domain.Request{
			Name:   "List users",
			Method: domain.MethodGet,
			URL:    server.URL + "/users",
		},
	}, responseStore, recorder)

	response, err := service.Send(t.TempDir(), "collections/users/list.yaml", "development")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	if len(recorder.entries) != 1 {
		t.Fatalf("expected one history entry, got %d", len(recorder.entries))
	}

	entry := recorder.entries[0]
	if entry.RequestPath != "collections/users/list.yaml" || entry.RequestName != "List users" {
		t.Errorf("unexpected history entry: %+v", entry)
	}
	if entry.StatusCode != http.StatusOK || entry.Environment != "development" {
		t.Errorf("unexpected history metadata: %+v", entry)
	}
	if entry.SizeBytes != int64(len("hello")) {
		t.Errorf("unexpected size %d", entry.SizeBytes)
	}
}

type fakeOAuth2Provider struct {
	token          string
	err            error
	calls          int
	authentication domain.Authentication
	layers         []map[string]string
}

func (provider *fakeOAuth2Provider) AccessToken(
	_ string,
	_ string,
	_ string,
	authentication domain.Authentication,
	layers []map[string]string,
) (string, error) {
	provider.calls++
	provider.authentication = authentication
	provider.layers = layers
	return provider.token, provider.err
}

func TestSendUsesOAuth2Provider(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	provider := &fakeOAuth2Provider{token: "provider-token"}
	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "List users",
			Method: domain.MethodGet,
			URL:    server.URL,
			Authentication: &domain.Authentication{
				Type:      domain.AuthenticationOAuth2,
				GrantType: domain.OAuth2GrantClientCredentials,
				ClientID:  "client-id",
				TokenURL:  "https://example.com/token",
			},
		},
	})
	SetOAuth2Provider(service, provider)

	response, err := service.Send(t.TempDir(), "collections/users/list.yaml", "development")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	if authorization != "Bearer provider-token" {
		t.Errorf("unexpected authorization %q", authorization)
	}
	if provider.calls != 1 {
		t.Errorf("expected one provider call, got %d", provider.calls)
	}
	if provider.authentication.GrantType != domain.OAuth2GrantClientCredentials {
		t.Errorf("unexpected authentication: %+v", provider.authentication)
	}
}

func TestSendRequiresOAuth2ProviderForGrant(t *testing.T) {
	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "List users",
			Method: domain.MethodGet,
			URL:    "https://example.com",
			Authentication: &domain.Authentication{
				Type:      domain.AuthenticationOAuth2,
				GrantType: domain.OAuth2GrantClientCredentials,
				ClientID:  "client-id",
				TokenURL:  "https://example.com/token",
			},
		},
	})

	_, err := service.Send(t.TempDir(), "collections/users/list.yaml", "")
	if !errors.Is(err, ErrOAuth2AuthorizationRequired) {
		t.Fatalf("expected ErrOAuth2AuthorizationRequired, got %v", err)
	}
}

type fakeScriptRunner struct {
	preRequestCalls   int
	postResponseCalls int
	preVariables      map[string]string
	postResponse      domain.ScriptResponse
}

func (runner *fakeScriptRunner) BeforeRequest(
	_ string,
	request *domain.Request,
	variables map[string]string,
) domain.ScriptOutcome {
	runner.preRequestCalls++
	variables["scriptValue"] = "from-script"
	runner.preVariables = variables
	request.Headers = append(request.Headers, domain.Header{
		Name:    "X-Script",
		Value:   "yes",
		Enabled: true,
	})

	return domain.ScriptOutcome{
		Logs:  []string{"preparing request"},
		Tests: []domain.ScriptTest{{Name: "pre test", Passed: true}},
	}
}

func (runner *fakeScriptRunner) AfterResponse(
	_ string,
	_ domain.Request,
	response domain.ScriptResponse,
	_ map[string]string,
) domain.ScriptOutcome {
	runner.postResponseCalls++
	runner.postResponse = response

	return domain.ScriptOutcome{
		Logs:  []string{"received response"},
		Tests: []domain.ScriptTest{{Name: "post test", Passed: true}},
	}
}

func TestSendRunsRequestScripts(t *testing.T) {
	var receivedPath string
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedPath = request.URL.Path
		receivedHeader = request.Header.Get("X-Script")
		_, _ = writer.Write([]byte("hello from the server"))
	}))
	defer server.Close()

	runner := &fakeScriptRunner{}
	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "List users",
			Method: domain.MethodGet,
			URL:    server.URL + "/{{scriptValue}}",
			Scripts: &domain.Scripts{
				PreRequest:   "request.headers['X-Script'] = 'yes'",
				PostResponse: "awake.log(response.body)",
			},
		},
	})
	SetScriptRunner(service, runner)

	response, err := service.Send(t.TempDir(), "collections/users/list.yaml", "")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	if runner.preRequestCalls != 1 {
		t.Errorf("expected one pre-request script, got %d", runner.preRequestCalls)
	}
	if runner.postResponseCalls != 1 {
		t.Errorf("expected one post-response script, got %d", runner.postResponseCalls)
	}
	if receivedPath != "/from-script" {
		t.Errorf("unexpected path: %q", receivedPath)
	}
	if receivedHeader != "yes" {
		t.Errorf("unexpected script header: %q", receivedHeader)
	}
	if response.PreRequestOutcome.Logs[0] != "preparing request" {
		t.Errorf("unexpected pre-request outcome: %+v", response.PreRequestOutcome)
	}
	if response.PostResponseOutcome.Tests[0].Name != "post test" {
		t.Errorf("unexpected post-response outcome: %+v", response.PostResponseOutcome)
	}
	if runner.postResponse.Body != "hello from the server" {
		t.Errorf("unexpected response preview: %q", runner.postResponse.Body)
	}
}

func TestSendSkipsScriptsWhenTheyAreEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	runner := &fakeScriptRunner{}
	service := newTestService(t, fakeSource{
		request: domain.Request{
			Name:   "List users",
			Method: domain.MethodGet,
			URL:    server.URL,
			Scripts: &domain.Scripts{
				PreRequest:   "  ",
				PostResponse: "",
			},
		},
	})
	SetScriptRunner(service, runner)

	response, err := service.Send(t.TempDir(), "collections/users/list.yaml", "")
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer service.Close(response.ID)

	if runner.preRequestCalls != 0 || runner.postResponseCalls != 0 {
		t.Errorf("expected no script calls, got %d and %d", runner.preRequestCalls, runner.postResponseCalls)
	}
}

func newTestService(t *testing.T, source Source) *Service {
	t.Helper()

	responseStore, err := newStoreAt(t.TempDir())
	if err != nil {
		t.Fatalf("create response store: %v", err)
	}

	return newServiceWithStore(source, responseStore, nil)
}
