package plugins

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/jsruntime"
)

const (
	// defaultLoadTimeout bounds the evaluation of a plugin bundle.
	defaultLoadTimeout = 5 * time.Second
	// defaultCallTimeout bounds one call into a plugin, not counting the
	// HTTP requests it waits on.
	defaultCallTimeout = 5 * time.Second
	// fetchTimeout bounds one request a plugin sends with awake.http.fetch.
	fetchTimeout = 30 * time.Second
	// maximumFetchBody bounds the response body a plugin reads.
	maximumFetchBody = 10 * 1024 * 1024
	// maximumAuthenticationBody bounds the request body an authentication
	// handler reads to sign it.
	maximumAuthenticationBody = 10 * 1024 * 1024

	timeoutReason = "plugin timed out"
)

var (
	// ErrPluginNotRegistered is returned for a bundle that never calls
	// awake.register.
	ErrPluginNotRegistered = errors.New("plugins: the plugin did not call awake.register")
	// ErrUndeclaredHandler is returned when a bundle registers a handler its
	// manifest does not declare, or misses one it declares. The manifest is
	// what the user approved, so the two must agree.
	ErrUndeclaredHandler = errors.New("plugins: the handlers do not match the manifest")
	// ErrAsynchronousResult is returned when a handler returns a promise that
	// is still pending: plugins run synchronously.
	ErrAsynchronousResult = errors.New("plugins: the handler returned a promise that never settled")
	// ErrRequestBodyTooLarge is returned when a request to authenticate has a
	// body larger than a handler may read.
	ErrRequestBodyTooLarge = errors.New("plugins: the request body is too large to authenticate")
)

// RuntimeOptions configures one plugin runtime.
type RuntimeOptions struct {
	// Settings are the values the plugin reads in awake.settings: its
	// defaults, overridden by the shared and secret values of the user.
	Settings map[string]string
	// HTTPClient sends the requests of awake.http.fetch. It is only used when
	// the manifest asks for the network permission.
	HTTPClient  *http.Client
	LoadTimeout time.Duration
	CallTimeout time.Duration
}

// CallLog holds what one call into a plugin printed.
type CallLog struct {
	Lines []string
}

// Runtime is one loaded plugin. goja runtimes are not safe for concurrent use,
// so calls are serialized.
type Runtime struct {
	manifest    Manifest
	options     RuntimeOptions
	mutex       sync.Mutex
	machine     *goja.Runtime
	handlers    *goja.Object
	registered  bool
	deadline    *jsruntime.Deadline
	currentLogs *[]string
}

// handlerGroups lists the keys of awake.register that hold one handler per
// contribution name.
var handlerGroups = []string{"dynamicVariables", "authentication", "importFormats", "exportFormats", "commands"}

// viewHandlersGroup holds, for each view, the handlers its page may call.
// Unlike the other groups a view needs no handler, but a handler needs a
// declared view.
const viewHandlersGroup = "views"

// LoadRuntime evaluates the bundle of a plugin in a fresh, isolated runtime.
// It has no module loader, file system or network of its own: everything goes
// through the awake object, filtered by the permissions of the manifest.
func LoadRuntime(manifest Manifest, bundle []byte, options RuntimeOptions) (*Runtime, error) {
	if options.LoadTimeout <= 0 {
		options.LoadTimeout = defaultLoadTimeout
	}
	if options.CallTimeout <= 0 {
		options.CallTimeout = defaultCallTimeout
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: fetchTimeout}
	}

	runtime := &Runtime{manifest: manifest, options: options, machine: goja.New()}
	runtime.install()

	logs := []string{}
	runtime.currentLogs = &logs
	runtime.deadline = jsruntime.NewDeadline(runtime.machine, options.LoadTimeout, timeoutReason)
	_, err := runtime.machine.RunScript(manifest.Main, string(bundle))
	runtime.deadline.Stop()
	runtime.currentLogs = nil
	if err != nil {
		return nil, fmt.Errorf("plugins: load %s: %w", manifest.ID, err)
	}
	if !runtime.registered {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotRegistered, manifest.ID)
	}
	if err := runtime.checkHandlers(); err != nil {
		return nil, err
	}
	return runtime, nil
}

// Manifest returns the manifest the runtime was loaded from.
func (runtime *Runtime) Manifest() Manifest {
	return runtime.manifest
}

// DynamicVariable computes the value of a {{$name}} placeholder.
func (runtime *Runtime) DynamicVariable(name string) (string, CallLog, error) {
	var value string
	log, err := runtime.call(func(machine *goja.Runtime) error {
		result, err := runtime.invoke("dynamicVariables", name, runtime.context(nil))
		if err != nil {
			return err
		}
		value = jsruntime.ValueToString(result)
		return nil
	})
	return value, log, err
}

// BeforeRequest lets the plugin change the request and its variables before
// the pre-request script of the request runs.
func (runtime *Runtime) BeforeRequest(request *domain.Request, variables map[string]string) (CallLog, error) {
	return runtime.call(func(machine *goja.Runtime) error {
		requestObject := jsruntime.RequestToObject(machine, request)
		variablesObject := jsruntime.StringMapToObject(machine, variables)
		context := runtime.context(map[string]any{"request": requestObject, "variables": variablesObject})
		if _, err := runtime.invokeHook(HookBeforeRequest, context); err != nil {
			return err
		}
		jsruntime.ApplyRequestObject(machine, requestObject, request)
		replaceStringMap(variables, jsruntime.ReadStringMap(machine, variablesObject))
		return nil
	})
}

// AfterResponse lets the plugin read the response and change the variables
// before the post-response script of the request runs.
func (runtime *Runtime) AfterResponse(request domain.Request, response domain.ScriptResponse, variables map[string]string) (CallLog, error) {
	return runtime.call(func(machine *goja.Runtime) error {
		variablesObject := jsruntime.StringMapToObject(machine, variables)
		context := runtime.context(map[string]any{
			"request":   jsruntime.RequestToObject(machine, &request),
			"response":  jsruntime.ResponseToObject(machine, response),
			"variables": variablesObject,
		})
		if _, err := runtime.invokeHook(HookAfterResponse, context); err != nil {
			return err
		}
		replaceStringMap(variables, jsruntime.ReadStringMap(machine, variablesObject))
		return nil
	})
}

// Authenticate lets the plugin sign the request about to be sent. It sees the
// final method, URL, headers and body, and may change the URL and headers.
// fields holds the values the request gives the authentication type, with
// their variables resolved.
func (runtime *Runtime) Authenticate(authenticationType string, httpRequest *http.Request, fields map[string]string) (CallLog, error) {
	body, err := readRequestBody(httpRequest)
	if err != nil {
		return CallLog{}, err
	}
	return runtime.call(func(machine *goja.Runtime) error {
		requestObject := machine.NewObject()
		_ = requestObject.Set("method", httpRequest.Method)
		_ = requestObject.Set("url", httpRequest.URL.String())
		_ = requestObject.Set("headers", jsruntime.StringMapToObject(machine, flattenHeaders(httpRequest.Header)))
		_ = requestObject.Set("body", body)
		context := runtime.context(map[string]any{
			"request": requestObject,
			"fields":  jsruntime.StringMapToObject(machine, fields),
		})
		if _, err := runtime.invoke("authentication", authenticationType, context); err != nil {
			return err
		}
		return applySignedRequest(machine, requestObject, httpRequest)
	})
}

// Import converts a document to collections, returned as the JSON the
// interop feature reads.
func (runtime *Runtime) Import(format string, fileName string, content string) ([]byte, CallLog, error) {
	var encoded []byte
	log, err := runtime.call(func(machine *goja.Runtime) error {
		context := runtime.context(map[string]any{"fileName": fileName, "content": content})
		result, err := runtime.invoke("importFormats", format, context)
		if err != nil {
			return err
		}
		encoded, err = json.Marshal(result.Export())
		if err != nil {
			return fmt.Errorf("plugins: encode the collections %s returned: %w", runtime.manifest.ID, err)
		}
		return nil
	})
	return encoded, log, err
}

// Export converts collections, given as the JSON the interop feature writes,
// to the content of a document.
func (runtime *Runtime) Export(format string, collections []byte) (string, CallLog, error) {
	var content string
	log, err := runtime.call(func(machine *goja.Runtime) error {
		parsed, err := parseJSON(machine, string(collections))
		if err != nil {
			return err
		}
		result, err := runtime.invoke("exportFormats", format, runtime.context(map[string]any{"collections": parsed}))
		if err != nil {
			return err
		}
		if _, isString := result.Export().(string); !isString {
			return fmt.Errorf("plugins: %s must return the document as a string", runtime.manifest.ID)
		}
		content = result.String()
		return nil
	})
	return content, log, err
}

// RunCommand runs a command of the command palette.
func (runtime *Runtime) RunCommand(commandID string) (CallLog, error) {
	return runtime.call(func(*goja.Runtime) error {
		_, err := runtime.invoke("commands", commandID, runtime.context(nil))
		return err
	})
}

// InvokeView calls a handler a view of the plugin registered, for the page of
// that view. The payload and the result travel as JSON, since the page runs
// in a sandboxed frame of the interface.
func (runtime *Runtime) InvokeView(viewID string, handler string, payload string) (string, CallLog, error) {
	var result string
	log, err := runtime.call(func(machine *goja.Runtime) error {
		views := runtime.handlers.Get(viewHandlersGroup)
		if jsruntime.IsAbsent(views) {
			return fmt.Errorf("%w: no view handler", ErrUndeclaredHandler)
		}
		handlers := views.ToObject(machine).Get(viewID)
		if jsruntime.IsAbsent(handlers) {
			return fmt.Errorf("%w: view %s has no handler", ErrUndeclaredHandler, viewID)
		}
		parsed := goja.Undefined()
		if payload != "" {
			value, err := parseJSON(machine, payload)
			if err != nil {
				return fmt.Errorf("plugins: the payload is not JSON: %w", err)
			}
			parsed = value
		}
		value, err := runtime.callHandler(handlers.ToObject(machine).Get(handler), viewHandlersGroup+"."+viewID+"."+handler, runtime.context(map[string]any{"payload": parsed}))
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(value.Export())
		if err != nil {
			return fmt.Errorf("plugins: encode the result of %s: %w", handler, err)
		}
		result = string(encoded)
		return nil
	})
	return result, log, err
}

// call runs one operation under the call timeout, collecting the logs it
// prints.
func (runtime *Runtime) call(operation func(machine *goja.Runtime) error) (CallLog, error) {
	runtime.mutex.Lock()
	defer runtime.mutex.Unlock()

	logs := []string{}
	runtime.currentLogs = &logs
	runtime.machine.ClearInterrupt()
	runtime.deadline = jsruntime.NewDeadline(runtime.machine, runtime.options.CallTimeout, timeoutReason)
	defer func() {
		runtime.deadline.Stop()
		runtime.currentLogs = nil
	}()

	err := operation(runtime.machine)
	if err != nil {
		err = fmt.Errorf("%s: %w", runtime.manifest.ID, err)
	}
	return CallLog{Lines: logs}, err
}

func (runtime *Runtime) invokeHook(hook Hook, context *goja.Object) (goja.Value, error) {
	handler := runtime.handlers.Get(string(hook))
	return runtime.callHandler(handler, string(hook), context)
}

func (runtime *Runtime) invoke(group string, name string, context *goja.Object) (goja.Value, error) {
	handlers := runtime.handlers.Get(group)
	if jsruntime.IsAbsent(handlers) {
		return nil, fmt.Errorf("%w: no %s handler", ErrUndeclaredHandler, group)
	}
	handler := handlers.ToObject(runtime.machine).Get(name)
	return runtime.callHandler(handler, group+"."+name, context)
}

// callHandler calls a handler and settles the promise an async handler
// returns, since goja runs pending jobs before returning.
func (runtime *Runtime) callHandler(handler goja.Value, label string, context *goja.Object) (goja.Value, error) {
	function, ok := goja.AssertFunction(handler)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not a function", ErrUndeclaredHandler, label)
	}
	result, err := function(goja.Undefined(), context)
	if err != nil {
		return nil, err
	}
	promise, isPromise := result.Export().(*goja.Promise)
	if !isPromise {
		return result, nil
	}
	switch promise.State() {
	case goja.PromiseStateFulfilled:
		return promise.Result(), nil
	case goja.PromiseStateRejected:
		return nil, fmt.Errorf("%s: %s", label, promise.Result().String())
	default:
		return nil, fmt.Errorf("%w: %s", ErrAsynchronousResult, label)
	}
}

// context builds the argument every handler receives: the settings of the
// plugin plus the values of the call.
func (runtime *Runtime) context(values map[string]any) *goja.Object {
	context := runtime.machine.NewObject()
	_ = context.Set("settings", jsruntime.StringMapToObject(runtime.machine, runtime.options.Settings))
	for name, value := range values {
		_ = context.Set(name, value)
	}
	return context
}

// install defines the awake object of the plugin.
func (runtime *Runtime) install() {
	machine := runtime.machine
	awake := machine.NewObject()
	sink := func(line string) {
		if runtime.currentLogs != nil {
			*runtime.currentLogs = append(*runtime.currentLogs, line)
		}
	}
	jsruntime.InstallConsole(machine, sink)
	_ = awake.Set("log", jsruntime.LogFunction(sink, ""))

	plugin := machine.NewObject()
	_ = plugin.Set("id", runtime.manifest.ID)
	_ = plugin.Set("version", runtime.manifest.PluginVersion)
	_ = awake.Set("plugin", plugin)
	_ = awake.Set("settings", jsruntime.StringMapToObject(machine, runtime.options.Settings))
	_ = awake.Set("register", runtime.register)
	_ = awake.Set("crypto", runtime.cryptoObject())
	_ = awake.Set("encoding", runtime.encodingObject())
	if runtime.manifest.HasPermission(PermissionNetwork) {
		http := machine.NewObject()
		_ = http.Set("fetch", runtime.fetch)
		_ = awake.Set("http", http)
	}
	_ = machine.Set("awake", awake)
}

func (runtime *Runtime) register(call goja.FunctionCall) goja.Value {
	if runtime.registered {
		panic(runtime.machine.NewTypeError("awake.register: call it once"))
	}
	argument := call.Argument(0)
	if jsruntime.IsAbsent(argument) {
		panic(runtime.machine.NewTypeError("awake.register: pass an object of handlers"))
	}
	runtime.handlers = argument.ToObject(runtime.machine)
	runtime.registered = true
	return goja.Undefined()
}

// checkHandlers compares the registered handlers with the contributions of
// the manifest, both ways.
func (runtime *Runtime) checkHandlers() error {
	contributions := runtime.manifest.Contributes
	expected := map[string][]string{
		"dynamicVariables": namesOf(contributions.DynamicVariables, func(item DynamicVariableContribution) string { return item.Name }),
		"authentication":   namesOf(contributions.Authentication, func(item AuthenticationContribution) string { return item.Type }),
		"importFormats":    namesOf(contributions.ImportFormats, func(item FormatContribution) string { return item.Format }),
		"exportFormats":    namesOf(contributions.ExportFormats, func(item FormatContribution) string { return item.Format }),
		"commands":         namesOf(contributions.Commands, func(item CommandContribution) string { return item.ID }),
	}

	problems := []string{}
	known := append(slices.Clone(handlerGroups), string(HookBeforeRequest), string(HookAfterResponse), viewHandlersGroup)
	for _, key := range runtime.handlers.Keys() {
		if !slices.Contains(known, key) {
			problems = append(problems, fmt.Sprintf("unknown handler group %q", key))
		}
	}
	for _, hook := range knownHooks {
		_, isFunction := goja.AssertFunction(runtime.handlers.Get(string(hook)))
		declared := slices.Contains(contributions.Hooks, hook)
		switch {
		case declared && !isFunction:
			problems = append(problems, fmt.Sprintf("hook %s is declared but not registered", hook))
		case !declared && !jsruntime.IsAbsent(runtime.handlers.Get(string(hook))):
			problems = append(problems, fmt.Sprintf("hook %s is registered but not declared", hook))
		}
	}
	for _, group := range handlerGroups {
		registered := []string{}
		if value := runtime.handlers.Get(group); !jsruntime.IsAbsent(value) {
			registered = value.ToObject(runtime.machine).Keys()
		}
		for _, name := range expected[group] {
			if !slices.Contains(registered, name) {
				problems = append(problems, fmt.Sprintf("%s %q is declared but not registered", group, name))
			}
		}
		for _, name := range registered {
			if !slices.Contains(expected[group], name) {
				problems = append(problems, fmt.Sprintf("%s %q is registered but not declared", group, name))
			}
		}
	}

	declaredViews := append(
		namesOf(contributions.ResponseViewers, func(item ResponseViewerContribution) string { return item.ID }),
		namesOf(contributions.RequestTabs, func(item RequestTabContribution) string { return item.ID })...,
	)
	if value := runtime.handlers.Get(viewHandlersGroup); !jsruntime.IsAbsent(value) {
		for _, name := range value.ToObject(runtime.machine).Keys() {
			if !slices.Contains(declaredViews, name) {
				problems = append(problems, fmt.Sprintf("views %q is registered but not declared", name))
			}
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%w: %s: %s", ErrUndeclaredHandler, runtime.manifest.ID, strings.Join(problems, "; "))
	}
	return nil
}

// fetch implements awake.http.fetch(url, { method, headers, body }). It is
// synchronous and does not count against the call timeout.
func (runtime *Runtime) fetch(call goja.FunctionCall) goja.Value {
	machine := runtime.machine
	target := strings.TrimSpace(call.Argument(0).String())
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		panic(machine.NewTypeError("awake.http.fetch: %q is not an http or https URL", target))
	}

	method := http.MethodGet
	headers := map[string]string{}
	var body io.Reader
	if options := call.Argument(1); !jsruntime.IsAbsent(options) {
		object := options.ToObject(machine)
		if value := object.Get("method"); !jsruntime.IsAbsent(value) {
			method = strings.ToUpper(value.String())
		}
		headers = jsruntime.ReadStringMap(machine, object.Get("headers"))
		if value := object.Get("body"); !jsruntime.IsAbsent(value) {
			body = strings.NewReader(jsruntime.ValueToString(value))
		}
	}

	request, err := http.NewRequest(method, parsed.String(), body)
	if err != nil {
		panic(machine.NewGoError(fmt.Errorf("awake.http.fetch: %w", err)))
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	runtime.deadline.Pause()
	startedAt := time.Now()
	response, err := runtime.options.HTTPClient.Do(request)
	var content []byte
	if err == nil {
		content, err = io.ReadAll(io.LimitReader(response.Body, maximumFetchBody))
		response.Body.Close()
	}
	runtime.deadline.Resume()
	if err != nil {
		panic(machine.NewGoError(fmt.Errorf("awake.http.fetch %s: %w", parsed.Redacted(), err)))
	}

	return jsruntime.ResponseToObject(machine, domain.ScriptResponse{
		StatusCode:           response.StatusCode,
		Status:               response.Status,
		Headers:              response.Header,
		Body:                 string(content),
		DurationMilliseconds: time.Since(startedAt).Milliseconds(),
	})
}

// cryptoObject offers the digests request signatures need, such as AWS
// Signature Version 4, which goja does not provide.
func (runtime *Runtime) cryptoObject() *goja.Object {
	machine := runtime.machine
	object := machine.NewObject()
	_ = object.Set("sha256", func(message string) string {
		sum := sha256.Sum256([]byte(message))
		return hex.EncodeToString(sum[:])
	})
	// hmacSha256 takes the key as text, or as hexadecimal when keyEncoding is
	// "hex", so a signing key derived from a previous digest can be chained.
	_ = object.Set("hmacSha256", func(key string, message string, keyEncoding goja.Value) string {
		keyBytes := []byte(key)
		if !jsruntime.IsAbsent(keyEncoding) && keyEncoding.String() == "hex" {
			decoded, err := hex.DecodeString(key)
			if err != nil {
				panic(machine.NewTypeError("awake.crypto.hmacSha256: the key is not hexadecimal"))
			}
			keyBytes = decoded
		}
		mac := hmac.New(sha256.New, keyBytes)
		mac.Write([]byte(message))
		return hex.EncodeToString(mac.Sum(nil))
	})
	return object
}

func (runtime *Runtime) encodingObject() *goja.Object {
	machine := runtime.machine
	object := machine.NewObject()
	_ = object.Set("base64Encode", func(text string) string {
		return base64.StdEncoding.EncodeToString([]byte(text))
	})
	_ = object.Set("base64Decode", func(text string) string {
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			panic(machine.NewTypeError("awake.encoding.base64Decode: the text is not base64"))
		}
		return string(decoded)
	})
	return object
}

// readRequestBody reads the body of a request about to be sent and puts it
// back, so the handler can sign it.
func readRequestBody(httpRequest *http.Request) (string, error) {
	if httpRequest.Body == nil || httpRequest.Body == http.NoBody {
		return "", nil
	}
	content, err := io.ReadAll(io.LimitReader(httpRequest.Body, maximumAuthenticationBody+1))
	httpRequest.Body.Close()
	if err != nil {
		return "", fmt.Errorf("plugins: read the request body: %w", err)
	}
	if len(content) > maximumAuthenticationBody {
		return "", ErrRequestBodyTooLarge
	}
	httpRequest.Body = io.NopCloser(strings.NewReader(string(content)))
	httpRequest.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(content))), nil
	}
	httpRequest.ContentLength = int64(len(content))
	return string(content), nil
}

// applySignedRequest copies the URL and headers the handler set back into the
// request. A header the handler deleted is removed.
func applySignedRequest(machine *goja.Runtime, object *goja.Object, httpRequest *http.Request) error {
	signedURL, err := url.Parse(object.Get("url").String())
	if err != nil {
		return fmt.Errorf("plugins: the signed URL is invalid: %w", err)
	}
	if signedURL.Host != httpRequest.URL.Host || signedURL.Scheme != httpRequest.URL.Scheme {
		return errors.New("plugins: an authentication handler may not send the request to another server")
	}
	httpRequest.URL = signedURL

	headers := jsruntime.ReadStringMap(machine, object.Get("headers"))
	httpRequest.Header = http.Header{}
	for name, value := range headers {
		httpRequest.Header.Set(name, value)
	}
	return nil
}

func flattenHeaders(header http.Header) map[string]string {
	flat := make(map[string]string, len(header))
	for name, values := range header {
		flat[name] = strings.Join(values, ", ")
	}
	return flat
}

func parseJSON(machine *goja.Runtime, text string) (goja.Value, error) {
	parse, _ := goja.AssertFunction(machine.Get("JSON").ToObject(machine).Get("parse"))
	return parse(goja.Undefined(), machine.ToValue(text))
}

func replaceStringMap(target map[string]string, values map[string]string) {
	clear(target)
	maps.Copy(target, values)
}

func namesOf[Item any](items []Item, name func(Item) string) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, name(item))
	}
	return names
}
