// Package scripts runs the JavaScript hooks attached to a request. The
// runtime is synchronous and bounded by a timeout so a faulty script cannot
// block a request forever.
package scripts

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/jsruntime"
)

const (
	scriptTimeout       = 5 * time.Second
	scriptTimeoutReason = "script timed out"
)

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
// precedence layer before interpolation. execute runs the requests the script
// asks for with awake.runRequest; it is nil when the feature is unavailable.
func (runner *Runner) BeforeRequest(script string, request *domain.Request, variables map[string]string, environments domain.EnvironmentResolver, execute domain.ScriptRequestExecutor) domain.ScriptOutcome {
	if strings.TrimSpace(script) == "" {
		return domain.ScriptOutcome{}
	}

	runtime := goja.New()
	outcome := domain.ScriptOutcome{}
	deadline := jsruntime.NewDeadline(runtime, runner.timeout, scriptTimeoutReason)
	requestObject := requestToObject(runtime, request)
	variablesObject := jsruntime.StringMapToObject(runtime, variables)
	installHelper(runtime, variablesObject, environments, &outcome, execute, deadline)
	installConsole(runtime, &outcome)

	runtime.Set("request", requestObject)
	runtime.Set("variables", variablesObject)

	run(runtime, script, deadline, &outcome)

	applyRequestObject(runtime, requestObject, request)
	readVariables(variablesObject, variables)

	return outcome
}

// AfterResponse runs the post-response script against the response preview.
func (runner *Runner) AfterResponse(script string, request domain.Request, response domain.ScriptResponse, variables map[string]string, environments domain.EnvironmentResolver) domain.ScriptOutcome {
	if strings.TrimSpace(script) == "" {
		return domain.ScriptOutcome{}
	}

	runtime := goja.New()
	outcome := domain.ScriptOutcome{}
	deadline := jsruntime.NewDeadline(runtime, runner.timeout, scriptTimeoutReason)
	variablesObject := jsruntime.StringMapToObject(runtime, variables)
	installHelper(runtime, variablesObject, environments, &outcome, nil, deadline)
	installConsole(runtime, &outcome)

	runtime.Set("request", requestToObject(runtime, &request))
	runtime.Set("response", responseToObject(runtime, response))
	runtime.Set("variables", variablesObject)

	run(runtime, script, deadline, &outcome)
	readVariables(variablesObject, variables)

	return outcome
}

func run(runtime *goja.Runtime, script string, deadline *jsruntime.Deadline, outcome *domain.ScriptOutcome) {
	defer deadline.Stop()

	if _, err := runtime.RunString(script); err != nil {
		outcome.Error = err.Error()
	}
}

func installHelper(runtime *goja.Runtime, variablesObject *goja.Object, environments domain.EnvironmentResolver, outcome *domain.ScriptOutcome, execute domain.ScriptRequestExecutor, deadline *jsruntime.Deadline) {
	helper := runtime.NewObject()

	_ = helper.Set("log", jsruntime.LogFunction(outcomeLogSink(outcome), ""))

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

	// Only the pre-request script may run another request, and only when the
	// caller connected an executor to it.
	if execute != nil {
		_ = helper.Set("runRequest", runRequestFunction(runtime, execute, deadline))
	}

	// The variables object is the single source of truth while the script
	// runs, so awake.setVariable and direct writes to variables agree.
	_ = helper.Set("getVariable", func(name string) goja.Value {
		return readVariable(variablesObject, name)
	})

	_ = helper.Set("setVariable", func(name string, value goja.Value) {
		_ = variablesObject.Set(name, jsruntime.ValueToString(value))
	})

	// The collection of the request saves what the script stores: the shared
	// variables, committed through git, and the local overrides on
	// collection.local.yaml, which stay on this machine. The local variables
	// object follows, like awake.environment.set.
	collection := runtime.NewObject()
	_ = collection.Set("setVariable", func(name string, value goja.Value) {
		text := jsruntime.ValueToString(value)
		_ = variablesObject.Set(name, text)
		outcome.CollectionChanges = append(outcome.CollectionChanges, domain.CollectionChange{Name: name, Value: text})
	})
	_ = collection.Set("unset", func(name string) {
		_ = variablesObject.Delete(name)
		outcome.CollectionChanges = append(outcome.CollectionChanges, domain.CollectionChange{Name: name, Removed: true})
	})

	local := runtime.NewObject()
	_ = local.Set("setVariable", func(name string, value goja.Value) {
		text := jsruntime.ValueToString(value)
		_ = variablesObject.Set(name, text)
		outcome.CollectionChanges = append(outcome.CollectionChanges, domain.CollectionChange{Name: name, Value: text, Local: true})
	})
	_ = local.Set("unset", func(name string) {
		_ = variablesObject.Delete(name)
		outcome.CollectionChanges = append(outcome.CollectionChanges, domain.CollectionChange{Name: name, Removed: true, Local: true})
	})
	_ = collection.Set("local", local)
	_ = helper.Set("collection", collection)

	environment := runtime.NewObject()
	_ = environment.Set("get", func(name string) goja.Value {
		return readVariable(variablesObject, name)
	})
	_ = environment.Set("set", func(name string, value goja.Value) {
		text := jsruntime.ValueToString(value)
		_ = variablesObject.Set(name, text)
		outcome.EnvironmentChanges = append(outcome.EnvironmentChanges, domain.EnvironmentChange{Name: name, Value: text})
	})
	_ = environment.Set("unset", func(name string) {
		_ = variablesObject.Delete(name)
		outcome.EnvironmentChanges = append(outcome.EnvironmentChanges, domain.EnvironmentChange{Name: name, Removed: true})
	})
	_ = helper.Set("environment", environment)

	named := runtime.NewObject()
	handles := map[string]*goja.Object{}
	_ = named.Set("get", func(name string) *goja.Object {
		if environments == nil {
			panic(runtime.NewGoError(errEnvironmentsUnavailable))
		}
		environment, err := environments.ResolveEnvironment(name)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if handle, found := handles[environment.FileName]; found {
			return handle
		}
		handle := namedEnvironment(runtime, environment, outcome)
		handles[environment.FileName] = handle
		return handle
	})
	_ = helper.Set("environments", named)

	_ = runtime.Set("awake", helper)
}

var errEnvironmentsUnavailable = errors.New("other environments are not available to this script")

// runRequestFunction implements awake.runRequest(path, options). The call is
// synchronous: the script receives the response once the request is over.
func runRequestFunction(runtime *goja.Runtime, execute domain.ScriptRequestExecutor, deadline *jsruntime.Deadline) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		pathValue := call.Argument(0)
		if goja.IsUndefined(pathValue) || goja.IsNull(pathValue) {
			panic(runtime.NewTypeError("awake.runRequest: a request path is required, such as collections/auth/login.yaml"))
		}
		path := strings.TrimSpace(pathValue.String())
		if path == "" {
			panic(runtime.NewTypeError("awake.runRequest: a request path is required, such as collections/auth/login.yaml"))
		}

		timeout, err := requestTimeout(runtime, call.Argument(1))
		if err != nil {
			panic(runtime.NewGoError(err))
		}

		// The nested exchange runs under its own timeout; the script waits for
		// it without spending its own budget.
		deadline.Pause()
		response, err := execute(path, timeout)
		deadline.Resume()
		if err != nil {
			panic(runtime.NewGoError(err))
		}

		return responseToObject(runtime, response)
	}
}

// requestTimeout reads the timeout option of awake.runRequest, in
// milliseconds, zero meaning the default of the HTTP client.
func requestTimeout(runtime *goja.Runtime, options goja.Value) (time.Duration, error) {
	if options == nil || goja.IsUndefined(options) || goja.IsNull(options) {
		return 0, nil
	}

	timeout := options.ToObject(runtime).Get("timeout")
	if timeout == nil || goja.IsUndefined(timeout) || goja.IsNull(timeout) {
		return 0, nil
	}

	milliseconds := timeout.ToInteger()
	if milliseconds <= 0 {
		return 0, errors.New("awake.runRequest: timeout must be a positive number of milliseconds")
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}

// namedEnvironment is the handle returned by awake.environments.get. Its
// changes are saved like those of awake.environment but do not affect the
// variables of the running request.
func namedEnvironment(runtime *goja.Runtime, environment domain.ScriptEnvironment, outcome *domain.ScriptOutcome) *goja.Object {
	current := make(map[string]string, len(environment.Variables))
	for name, value := range environment.Variables {
		current[name] = value
	}
	change := func(name string) domain.EnvironmentChange {
		return domain.EnvironmentChange{Environment: environment.FileName, EnvironmentName: environment.Name, Name: name}
	}

	handle := runtime.NewObject()
	_ = handle.Set("name", environment.Name)
	_ = handle.Set("fileName", environment.FileName)
	_ = handle.Set("get", func(name string) goja.Value {
		value, found := current[name]
		if !found {
			return goja.Null()
		}
		return runtime.ToValue(value)
	})
	_ = handle.Set("set", func(name string, value goja.Value) {
		text := jsruntime.ValueToString(value)
		current[name] = text
		saved := change(name)
		saved.Value = text
		outcome.EnvironmentChanges = append(outcome.EnvironmentChanges, saved)
	})
	_ = handle.Set("unset", func(name string) {
		delete(current, name)
		removed := change(name)
		removed.Removed = true
		outcome.EnvironmentChanges = append(outcome.EnvironmentChanges, removed)
	})
	return handle
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

// readVariables copies the variables object back into the map, dropping the
// names the script deleted.
func readVariables(object *goja.Object, variables map[string]string) {
	clear(variables)
	for _, name := range object.Keys() {
		variables[name] = jsruntime.ValueToString(object.Get(name))
	}
}

func readVariable(object *goja.Object, name string) goja.Value {
	value := object.Get(name)
	if value == nil || goja.IsUndefined(value) {
		return goja.Null()
	}
	return value
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

	headers := jsruntime.ReadStringMap(runtime, object.Get("headers"))
	request.Headers = mergeHeaders(request.Headers, headers)

	query := jsruntime.ReadStringMap(runtime, object.Get("query"))
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

// installConsole sends the console output of a script to its logs.
func installConsole(runtime *goja.Runtime, outcome *domain.ScriptOutcome) {
	jsruntime.InstallConsole(runtime, outcomeLogSink(outcome))
}

func outcomeLogSink(outcome *domain.ScriptOutcome) jsruntime.LogSink {
	return func(line string) {
		outcome.Logs = append(outcome.Logs, line)
	}
}
