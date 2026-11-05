// Package scripts runs the JavaScript hooks attached to a request. The
// runtime is synchronous and bounded by a timeout so a faulty script cannot
// block a request forever.
package scripts

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/clementherve/awake/src/domain"
)

const scriptTimeout = 5 * time.Second

// Runner executes request scripts. The timeout bounds one script so a faulty
// hook cannot block a request forever.
type Runner struct {
	timeout time.Duration
}

// NewRunner returns a script runner with the default timeout.
func NewRunner() *Runner {
	return &Runner{timeout: scriptTimeout}
}

// BeforeRequest runs the pre-request script. It may mutate the request and
// the variable map; the caller applies the variable map as the highest
// precedence layer before interpolation.
func (runner *Runner) BeforeRequest(script string, request *domain.Request, variables map[string]string) domain.ScriptOutcome {
	if strings.TrimSpace(script) == "" {
		return domain.ScriptOutcome{}
	}

	runtime := goja.New()
	outcome := domain.ScriptOutcome{}
	requestObject := requestToObject(runtime, request)
	variablesObject := variablesToObject(runtime, variables)
	installHelper(runtime, variables, &outcome)

	runtime.Set("request", requestObject)
	runtime.Set("variables", variablesObject)

	run(runtime, script, runner.timeout, &outcome)

	applyRequestObject(runtime, requestObject, request)
	readVariables(variablesObject, variables)

	return outcome
}

// AfterResponse runs the post-response script against the response preview.
func (runner *Runner) AfterResponse(script string, request domain.Request, response domain.ScriptResponse, variables map[string]string) domain.ScriptOutcome {
	if strings.TrimSpace(script) == "" {
		return domain.ScriptOutcome{}
	}

	runtime := goja.New()
	outcome := domain.ScriptOutcome{}
	variablesObject := variablesToObject(runtime, variables)
	installHelper(runtime, variables, &outcome)

	runtime.Set("request", requestToObject(runtime, &request))
	runtime.Set("response", responseToObject(runtime, response))
	runtime.Set("variables", variablesObject)

	run(runtime, script, runner.timeout, &outcome)
	readVariables(variablesObject, variables)

	return outcome
}

func run(runtime *goja.Runtime, script string, timeout time.Duration, outcome *domain.ScriptOutcome) {
	timer := time.AfterFunc(timeout, func() {
		runtime.Interrupt("script timed out")
	})
	defer timer.Stop()

	if _, err := runtime.RunString(script); err != nil {
		outcome.Error = err.Error()
	}
}

func installHelper(runtime *goja.Runtime, variables map[string]string, outcome *domain.ScriptOutcome) {
	helper := runtime.NewObject()

	_ = helper.Set("log", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, 0, len(call.Arguments))
		for _, argument := range call.Arguments {
			parts = append(parts, valueToString(argument))
		}
		outcome.Logs = append(outcome.Logs, strings.Join(parts, " "))
		return goja.Undefined()
	})

	_ = helper.Set("test", func(call goja.FunctionCall) goja.Value {
		name := "test"
		if len(call.Arguments) > 0 {
			name = call.Arguments[0].String()
		}

		test := domain.ScriptTest{Name: name, Passed: true}
		if len(call.Arguments) > 1 {
			value := call.Arguments[1]
			if function, ok := goja.AssertFunction(value); ok {
				result, err := function(goja.Undefined())
				switch {
				case err != nil:
					test.Passed = false
					test.Error = err.Error()
				case result != nil &&
					!goja.IsUndefined(result) &&
					!goja.IsNull(result) &&
					!result.ToBoolean():
					test.Passed = false
					test.Error = "assertion returned false"
				}
			} else if !value.ToBoolean() {
				test.Passed = false
				test.Error = "assertion failed"
			}
		}

		outcome.Tests = append(outcome.Tests, test)
		return goja.Undefined()
	})

	_ = helper.Set("getVariable", func(name string) goja.Value {
		value, found := variables[name]
		if !found {
			return goja.Null()
		}
		return runtime.ToValue(value)
	})

	_ = helper.Set("setVariable", func(name string, value string) {
		variables[name] = value
	})

	_ = runtime.Set("awake", helper)
}

func requestToObject(runtime *goja.Runtime, request *domain.Request) *goja.Object {
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

func responseToObject(runtime *goja.Runtime, response domain.ScriptResponse) *goja.Object {
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

	return object
}

func variablesToObject(runtime *goja.Runtime, variables map[string]string) *goja.Object {
	object := runtime.NewObject()
	for name, value := range variables {
		_ = object.Set(name, value)
	}
	return object
}

func readVariables(object *goja.Object, variables map[string]string) {
	for _, name := range object.Keys() {
		variables[name] = object.Get(name).String()
	}
}

func applyRequestObject(runtime *goja.Runtime, object *goja.Object, request *domain.Request) {
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

	headers := readStringMap(runtime, object.Get("headers"))
	request.Headers = mergeHeaders(request.Headers, headers)

	query := readStringMap(runtime, object.Get("query"))
	request.Parameters = mergeParameters(request.Parameters, query)
}

func readStringMap(runtime *goja.Runtime, value goja.Value) map[string]string {
	result := map[string]string{}
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return result
	}

	object := value.ToObject(runtime)
	for _, name := range object.Keys() {
		result[name] = object.Get(name).String()
	}
	return result
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

func valueToString(value goja.Value) string {
	exported := value.Export()
	switch typed := exported.(type) {
	case string:
		return typed
	case nil:
		return "null"
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}
