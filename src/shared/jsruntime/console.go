package jsruntime

import (
	"strings"

	"github.com/dop251/goja"
)

const (
	warningPrefix = "warning: "
	errorPrefix   = "error: "
)

// LogSink receives one formatted log line.
type LogSink func(line string)

// InstallConsole maps console methods to sink, as goja provides no console of
// its own. Warnings and errors are prefixed to stand out.
func InstallConsole(runtime *goja.Runtime, sink LogSink) {
	console := runtime.NewObject()
	_ = console.Set("log", LogFunction(sink, ""))
	_ = console.Set("info", LogFunction(sink, ""))
	_ = console.Set("debug", LogFunction(sink, ""))
	_ = console.Set("warn", LogFunction(sink, warningPrefix))
	_ = console.Set("error", LogFunction(sink, errorPrefix))
	_ = runtime.Set("console", console)
}

// LogFunction returns a JavaScript function that joins its arguments, as
// console.log does, and hands the line to sink.
func LogFunction(sink LogSink, prefix string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		parts := make([]string, 0, len(call.Arguments))
		for _, argument := range call.Arguments {
			parts = append(parts, ValueToString(argument))
		}
		sink(prefix + strings.Join(parts, " "))
		return goja.Undefined()
	}
}
