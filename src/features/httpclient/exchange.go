package httpclient

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/clementherve/awake/src/domain"
)

// historyBodyLimit caps the request body kept in the history, like the
// response preview.
const historyBodyLimit = PreviewSize

const redacted = "•••"

// sentRequest is what actually left the application, after interpolation and
// authentication.
type sentRequest struct {
	method  string
	url     string
	headers http.Header
	body    string
}

// captureSentRequest copies the method, URL, headers and body of a prepared
// request before it is sent. Credentials are redacted.
func captureSentRequest(httpRequest *http.Request, authentication domain.Authentication) sentRequest {
	sent := sentRequest{
		method:  httpRequest.Method,
		url:     redactURL(httpRequest.URL, authentication),
		headers: redactHeaders(httpRequest.Header, authentication),
	}
	if httpRequest.GetBody != nil {
		if body, err := httpRequest.GetBody(); err == nil {
			content, _ := io.ReadAll(io.LimitReader(body, historyBodyLimit))
			body.Close()
			sent.body = string(content)
		}
	}
	return sent
}

// exchangeOf builds the history payload of a sent request and its response.
func exchangeOf(sent sentRequest, response Response) *domain.HistoryExchange {
	return &domain.HistoryExchange{
		RequestHeaders:    sent.headers,
		RequestBody:       sent.body,
		ResponseHeaders:   redactHeaders(response.Headers, domain.Authentication{}),
		ResponseBody:      response.Preview,
		ResponseEncoding:  response.Encoding,
		ResponseTruncated: response.Truncated || int64(len(response.Preview)) < response.Size,
	}
}

var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
}

// redactHeaders masks credentials, keeping the authorization scheme so the
// history still tells which kind of authentication was used.
func redactHeaders(headers map[string][]string, authentication domain.Authentication) map[string][]string {
	apiKeyHeader := ""
	if authentication.Type == domain.AuthenticationAPIKey && authentication.Location != "query" {
		apiKeyHeader = strings.ToLower(authentication.Key)
	}

	result := make(map[string][]string, len(headers))
	for name, values := range headers {
		lower := strings.ToLower(name)
		if !sensitiveHeaders[lower] && lower != apiKeyHeader {
			result[name] = append([]string(nil), values...)
			continue
		}
		masked := make([]string, 0, len(values))
		for _, value := range values {
			if scheme, _, found := strings.Cut(value, " "); found && (lower == "authorization" || lower == "proxy-authorization") {
				masked = append(masked, scheme+" "+redacted)
			} else {
				masked = append(masked, redacted)
			}
		}
		result[name] = masked
	}
	return result
}

// redactURL masks an API key sent in the query string.
func redactURL(address *url.URL, authentication domain.Authentication) string {
	if authentication.Type != domain.AuthenticationAPIKey || authentication.Location != "query" || authentication.Key == "" {
		return address.String()
	}
	copied := *address
	query := copied.Query()
	if query.Has(authentication.Key) {
		query.Set(authentication.Key, redacted)
		copied.RawQuery = query.Encode()
	}
	return copied.String()
}
