package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/variables"
)

func TestSendRunsPluginHooksDynamicVariablesAndAuthentication(t *testing.T) {
	var received *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received = request
		_, _ = writer.Write([]byte(`ok`))
	}))
	defer server.Close()

	host := &fakePluginHost{dynamic: map[string]string{"$stamp": "20261125"}, baseURL: server.URL}
	service := newTestService(t, labeledSource{
		request: domain.Request{
			Name:   "Signed",
			Method: domain.MethodGet,
			URL:    "{{baseUrl}}/items/{{$stamp}}",
			Authentication: &domain.Authentication{
				Type:   domain.PluginAuthenticationType("signer", "hmac"),
				Fields: map[string]string{"key": "{{apiKey}}"},
			},
			Body: &domain.Body{Type: domain.BodyNone},
		},
		layers: []variables.Layer{
			{Values: map[string]string{"apiKey": "public"}},
			{Values: map[string]string{"password": "hunter2"}, Secret: true},
		},
	})
	SetPluginHost(service, host)

	response, err := service.Send("/workspace", "collections/signed.yaml", "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if received.URL.Path != "/items/20261125" {
		t.Errorf("expected the hook variable and the dynamic value in the URL, got %s", received.URL.Path)
	}
	if received.Header.Get("X-Hook") != "before" || received.Header.Get("X-Signature") != "signed-with-public" {
		t.Errorf("expected the hook header and the signature, got %v", received.Header)
	}
	if !slices.Equal(host.secretNames, []string{"password"}) {
		t.Errorf("expected the secret names to reach the host, got %v", host.secretNames)
	}
	if !slices.Equal(host.dynamicNames, []string{"$stamp"}) {
		t.Errorf("expected only the plugin placeholder to be asked for, got %v", host.dynamicNames)
	}
	expectedLogs := []string{"[hook] before", "[dynamic] computed", "[signer] signed"}
	if !slices.Equal(response.PreRequestOutcome.Logs, expectedLogs) {
		t.Errorf("unexpected pre-request logs %v", response.PreRequestOutcome.Logs)
	}
	if !slices.Equal(response.PostResponseOutcome.Logs, []string{"[hook] after 200"}) {
		t.Errorf("unexpected post-response logs %v", response.PostResponseOutcome.Logs)
	}
}

func TestSendStopsWhenThePluginAuthenticationFails(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()

	request := domain.Request{
		Name:           "Signed",
		Method:         domain.MethodGet,
		URL:            server.URL,
		Authentication: &domain.Authentication{Type: domain.PluginAuthenticationType("signer", "hmac")},
		Body:           &domain.Body{Type: domain.BodyNone},
	}

	withoutHost := newTestService(t, fakeSource{request: request})
	if _, err := withoutHost.Send("/workspace", "signed.yaml", ""); !errors.Is(err, ErrPluginsUnavailable) {
		t.Errorf("expected ErrPluginsUnavailable without a host, got %v", err)
	}

	failing := newTestService(t, fakeSource{request: request})
	SetPluginHost(failing, &fakePluginHost{authenticationError: errors.New("plugin not enabled")})
	if _, err := failing.Send("/workspace", "signed.yaml", ""); err == nil || !strings.Contains(err.Error(), "plugin not enabled") {
		t.Errorf("expected the authentication error, got %v", err)
	}
	if called {
		t.Error("an unsigned request must not be sent")
	}
}

func TestSendReportsAPlaceholderNoPluginProvides(t *testing.T) {
	service := newTestService(t, fakeSource{request: domain.Request{
		Name: "Unknown", Method: domain.MethodGet, URL: "https://example.com/{{$unknown}}", Body: &domain.Body{Type: domain.BodyNone},
	}})
	SetPluginHost(service, &fakePluginHost{})

	_, err := service.Send("/workspace", "unknown.yaml", "")

	var missing *variables.MissingVariablesError
	if !errors.As(err, &missing) || missing.Names[0] != "$unknown" {
		t.Fatalf("expected $unknown to be missing, got %v", err)
	}
}

func TestCopyAsCurlIncludesThePluginSignature(t *testing.T) {
	service := newTestService(t, fakeSource{request: domain.Request{
		Name:           "Signed",
		Method:         domain.MethodGet,
		URL:            "https://example.com/{{$stamp}}",
		Authentication: &domain.Authentication{Type: domain.PluginAuthenticationType("signer", "hmac")},
		Body:           &domain.Body{Type: domain.BodyNone},
	}})
	SetPluginHost(service, &fakePluginHost{dynamic: map[string]string{"$stamp": "today"}})

	command, err := service.CopyAsCurl("/workspace", "signed.yaml", "")
	if err != nil {
		t.Fatalf("copy as curl: %v", err)
	}
	if !strings.Contains(command, "https://example.com/today") || !strings.Contains(command, "X-Signature: signed-with-") {
		t.Errorf("unexpected command %s", command)
	}
}

// labeledSource serves one request with labeled variable layers, so a test
// can mark one of them as read from the keychain.
type labeledSource struct {
	request domain.Request
	layers  []variables.Layer
}

func (source labeledSource) LoadRequest(string, string) (domain.Request, error) {
	return source.request, nil
}

func (source labeledSource) LoadVariableLayers(string, string, string) ([]variables.Layer, error) {
	return source.layers, nil
}

// fakePluginHost behaves like one plugin of each kind: a hook adding a header
// and the base URL, a dynamic variable and a signing authentication.
type fakePluginHost struct {
	dynamic             map[string]string
	authenticationError error
	secretNames         []string
	dynamicNames        []string
	baseURL             string
}

func (host *fakePluginHost) BeforeRequest(_ string, request *domain.Request, values map[string]string, secretNames []string) []string {
	host.secretNames = secretNames
	request.Headers = append(request.Headers, domain.Header{Name: "X-Hook", Value: "before", Enabled: true})
	if host.baseURL != "" {
		values["baseUrl"] = host.baseURL
	}
	return []string{"[hook] before"}
}

func (host *fakePluginHost) AfterResponse(_ string, _ domain.Request, response domain.ScriptResponse, _ map[string]string, _ []string) []string {
	return []string{"[hook] after " + strconv.Itoa(response.StatusCode)}
}

func (host *fakePluginHost) DynamicValues(_ string, names []string) (map[string]string, []string, error) {
	host.dynamicNames = names
	values := map[string]string{}
	for _, name := range names {
		if value, found := host.dynamic[name]; found {
			values[name] = value
		}
	}
	return values, []string{"[dynamic] computed"}, nil
}

func (host *fakePluginHost) Authenticate(_ string, pluginID string, _ string, httpRequest *http.Request, fields map[string]string) ([]string, error) {
	if host.authenticationError != nil {
		return nil, host.authenticationError
	}
	httpRequest.Header.Set("X-Signature", "signed-with-"+fields["key"])
	return []string{"[" + pluginID + "] signed"}, nil
}
