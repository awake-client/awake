package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPluginViewMiddlewareServesViewsWithASandboxPolicy(t *testing.T) {
	handler := pluginViewMiddleware(fakeViewFiles{})(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("application"))
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/plugin-views/key/commit/ui/view.html", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "<p>view</p>" {
		t.Fatalf("unexpected view response %d %q", recorder.Code, recorder.Body.String())
	}
	policy := recorder.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"connect-src 'none'", "form-action 'none'", "frame-src 'none'", "default-src 'none'"} {
		if !strings.Contains(policy, directive) {
			t.Errorf("expected %q in the policy %q", directive, policy)
		}
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected nosniff")
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/plugin-views/key/commit/missing.html", nil))
	if missing.Code != http.StatusNotFound {
		t.Errorf("expected a missing view to be not found, got %d", missing.Code)
	}

	posted := httptest.NewRecorder()
	handler.ServeHTTP(posted, httptest.NewRequest(http.MethodPost, "/plugin-views/key/commit/ui/view.html", nil))
	if posted.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected views to be read only, got %d", posted.Code)
	}

	application := httptest.NewRecorder()
	handler.ServeHTTP(application, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if application.Body.String() != "application" {
		t.Errorf("other paths must reach the application, got %q", application.Body.String())
	}
}

func TestPluginViewMiddlewareKeepsSandboxedFramesAwayFromTheRuntime(t *testing.T) {
	reached := false
	handler := pluginViewMiddleware(fakeViewFiles{})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}))

	fromFrame := httptest.NewRequest(http.MethodPost, "/wails/runtime", strings.NewReader(`{}`))
	fromFrame.Header.Set("Origin", "null")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, fromFrame)
	if recorder.Code != http.StatusForbidden || reached {
		t.Fatalf("expected a runtime call from an opaque origin to be refused, got %d", recorder.Code)
	}

	fromApplication := httptest.NewRequest(http.MethodPost, "/wails/runtime", strings.NewReader(`{}`))
	fromApplication.Header.Set("Origin", "wails://localhost")
	handler.ServeHTTP(httptest.NewRecorder(), fromApplication)
	if !reached {
		t.Error("runtime calls of the application must go through")
	}
}

type fakeViewFiles struct{}

func (fakeViewFiles) ViewFile(urlPath string) ([]byte, string, error) {
	if urlPath == "/plugin-views/key/commit/ui/view.html" {
		return []byte("<p>view</p>"), "text/html; charset=utf-8", nil
	}
	return nil, "", errors.New("not found")
}
