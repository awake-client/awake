package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/awake-client/awake/src/shared/variables"
)

const curlLineSeparator = " \\\n  "

// CopyAsCurl returns the request as a curl command, with its variables,
// headers, authentication and body resolved as Send would. Pre-request scripts
// are not run, since they may save variables in an environment. An OAuth 2
// token is obtained as for Send, so a flow that needs the user must be
// authorized first.
func (service *Service) CopyAsCurl(workspaceRoot string, requestPath string, environmentFileName string) (string, error) {
	request, err := service.source.LoadRequest(workspaceRoot, requestPath)
	if err != nil {
		return "", err
	}

	labeledLayers, err := service.source.LoadVariableLayers(workspaceRoot, requestPath, environmentFileName)
	if err != nil {
		return "", err
	}
	layers := variables.Maps(labeledLayers)

	// The environment may choose the authentication of the request, as for
	// Send. Pre-request scripts are skipped, since they may save variables.
	if err := applyEnvironmentAuthentication(&request, layers); err != nil {
		return "", err
	}

	oauth2Token, err := service.oauth2Token(workspaceRoot, requestPath, environmentFileName, request, layers)
	if err != nil {
		return "", err
	}

	httpRequest, err := buildHTTPRequestWithContext(context.Background(), request, layers, oauth2Token)
	if err != nil {
		return "", err
	}

	return formatCurl(httpRequest)
}

func formatCurl(httpRequest *http.Request) (string, error) {
	body, err := readRequestBody(httpRequest)
	if err != nil {
		return "", err
	}

	parts := []string{"curl"}
	if httpRequest.Method != http.MethodGet || body != "" {
		parts[0] += " -X " + httpRequest.Method
	}
	parts[0] += " " + quoteShell(httpRequest.URL.String())

	names := make([]string, 0, len(httpRequest.Header))
	for name := range httpRequest.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range httpRequest.Header[name] {
			parts = append(parts, "-H "+quoteShell(name+": "+value))
		}
	}

	if body != "" {
		parts = append(parts, "--data-raw "+quoteShell(body))
	}

	return strings.Join(parts, curlLineSeparator), nil
}

func readRequestBody(httpRequest *http.Request) (string, error) {
	if httpRequest.Body == nil {
		return "", nil
	}
	defer httpRequest.Body.Close()

	content, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		return "", fmt.Errorf("read request body: %w", err)
	}
	return string(content), nil
}

// quoteShell wraps value in single quotes for a POSIX shell, where nothing is
// special inside them except the single quote itself.
func quoteShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
