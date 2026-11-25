package jsruntime

import (
	"net/http"
	"sort"
	"strings"

	"github.com/dop251/goja"

	"github.com/awake-client/awake/src/domain"
)

// RequestToObject exposes the method, URL, enabled headers and query
// parameters, and body of a request as a JavaScript object a script may change.
func RequestToObject(runtime *goja.Runtime, request *domain.Request) *goja.Object {
	object := runtime.NewObject()
	_ = object.Set("method", request.Method)
	_ = object.Set("url", request.URL)

	headers := runtime.NewObject()
	for _, header := range request.Headers {
		if header.Enabled {
			_ = headers.Set(header.Name, header.Value)
		}
	}
	_ = object.Set("headers", headers)

	query := runtime.NewObject()
	for _, parameter := range request.Parameters {
		if parameter.Enabled {
			_ = query.Set(parameter.Name, parameter.Value)
		}
	}
	_ = object.Set("query", query)

	body := ""
	if request.Body != nil {
		body = request.Body.Content
	}
	_ = object.Set("body", body)

	return object
}

// ResponseToObject exposes a response preview, with its headers, cookies and a
// json() helper, as a JavaScript object.
func ResponseToObject(runtime *goja.Runtime, response domain.ScriptResponse) *goja.Object {
	object := runtime.NewObject()
	_ = object.Set("status", response.StatusCode)
	_ = object.Set("statusText", response.Status)
	_ = object.Set("body", response.Body)
	_ = object.Set("encoding", response.Encoding)
	_ = object.Set("duration", response.DurationMilliseconds)

	headers := runtime.NewObject()
	for name, values := range response.Headers {
		_ = headers.Set(name, strings.Join(values, ", "))
	}
	_ = object.Set("headers", headers)

	cookies := runtime.NewObject()
	pairs := make([]string, 0)
	for _, cookie := range (&http.Response{Header: http.Header(response.Headers)}).Cookies() {
		_ = cookies.Set(cookie.Name, cookie.Value)
		pairs = append(pairs, cookie.Name+"="+cookie.Value)
	}
	_ = object.Set("cookies", cookies)
	_ = object.Set("cookieHeader", strings.Join(pairs, "; "))

	// json parses the body preview, throwing a JavaScript error when it is not
	// valid JSON, like JSON.parse.
	_ = object.Set("json", func(goja.FunctionCall) goja.Value {
		parse, _ := goja.AssertFunction(runtime.Get("JSON").ToObject(runtime).Get("parse"))
		value, err := parse(goja.Undefined(), runtime.ToValue(response.Body))
		if err != nil {
			panic(err)
		}
		return value
	})

	return object
}

// ApplyRequestObject copies the changes a script made to the object back into
// the request. Disabled headers and parameters are kept as they were.
func ApplyRequestObject(runtime *goja.Runtime, object *goja.Object, request *domain.Request) {
	request.Method = strings.TrimSpace(object.Get("method").String())
	request.URL = object.Get("url").String()

	if body := object.Get("body"); body != nil && !goja.IsUndefined(body) && !goja.IsNull(body) {
		if request.Body == nil {
			request.Body = &domain.Body{Type: domain.BodyText}
		}
		request.Body.Content = body.String()
		if request.Body.Type == domain.BodyNone && request.Body.Content != "" {
			request.Body.Type = domain.BodyText
		}
	}

	headers := ReadStringMap(runtime, object.Get("headers"))
	request.Headers = mergeHeaders(request.Headers, headers)

	query := ReadStringMap(runtime, object.Get("query"))
	request.Parameters = mergeParameters(request.Parameters, query)
}

func mergeHeaders(existing []domain.Header, updated map[string]string) []domain.Header {
	result := make([]domain.Header, 0, len(existing)+len(updated))
	for _, header := range existing {
		if !header.Enabled {
			result = append(result, header)
			continue
		}
		value, found := updated[header.Name]
		if !found {
			continue
		}
		header.Value = value
		result = append(result, header)
		delete(updated, header.Name)
	}

	for _, name := range sortedKeys(updated) {
		result = append(result, domain.Header{Name: name, Value: updated[name], Enabled: true})
	}

	return result
}

func mergeParameters(existing []domain.Parameter, updated map[string]string) []domain.Parameter {
	result := make([]domain.Parameter, 0, len(existing)+len(updated))
	for _, parameter := range existing {
		if !parameter.Enabled {
			result = append(result, parameter)
			continue
		}
		value, found := updated[parameter.Name]
		if !found {
			continue
		}
		parameter.Value = value
		result = append(result, parameter)
		delete(updated, parameter.Name)
	}

	for _, name := range sortedKeys(updated) {
		result = append(result, domain.Parameter{Name: name, Value: updated[name], Enabled: true})
	}

	return result
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}
