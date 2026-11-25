package app

import (
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/awake-client/awake/src/features/plugins"
)

// pluginViewPolicy is the content security policy of a plugin view. The view
// runs in a frame sandboxed without same-origin access; the policy also
// forbids it any network request, form, nested frame or base URL, so it can
// only talk to the interface through postMessage.
const pluginViewPolicy = "default-src 'none'; " +
	"script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; media-src 'self' data: blob:; " +
	"connect-src 'none'; form-action 'none'; frame-src 'none'; child-src 'none'; " +
	"worker-src 'none'; base-uri 'none'; frame-ancestors 'self'"

// runtimePathPrefix starts the paths of the Wails runtime and bindings.
const runtimePathPrefix = "/wails/"

// opaqueOrigin is the Origin header a sandboxed frame sends.
const opaqueOrigin = "null"

// viewFiles serves the files of plugin views.
type viewFiles interface {
	ViewFile(urlPath string) ([]byte, string, error)
}

// pluginViewMiddleware serves plugin views and keeps them away from the Wails
// runtime: outside server mode Wails accepts every runtime call, so a request
// from an opaque origin, which only a sandboxed frame has, is refused.
func pluginViewMiddleware(views viewFiles) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			path := request.URL.Path
			if strings.HasPrefix(path, runtimePathPrefix) && request.Header.Get("Origin") == opaqueOrigin {
				http.Error(writer, "plugin views cannot call the application", http.StatusForbidden)
				return
			}
			if !strings.HasPrefix(path, plugins.ViewPathPrefix) {
				next.ServeHTTP(writer, request)
				return
			}
			if request.Method != http.MethodGet && request.Method != http.MethodHead {
				http.Error(writer, "plugin views are read only", http.StatusMethodNotAllowed)
				return
			}

			content, mediaType, err := views.ViewFile(path)
			if err != nil {
				http.NotFound(writer, request)
				return
			}
			headers := writer.Header()
			headers.Set("Content-Type", mediaType)
			headers.Set("Content-Security-Policy", pluginViewPolicy)
			headers.Set("X-Content-Type-Options", "nosniff")
			headers.Set("Referrer-Policy", "no-referrer")
			headers.Set("Cache-Control", "no-store")
			if request.Method == http.MethodHead {
				return
			}
			_, _ = writer.Write(content)
		})
	}
}
