package jsruntime

import (
	"encoding/json"
	"fmt"

	"github.com/dop251/goja"
)

// IsAbsent reports whether value is missing, undefined or null.
func IsAbsent(value goja.Value) bool {
	return value == nil || goja.IsUndefined(value) || goja.IsNull(value)
}

// ValueToString converts a JavaScript value to the text a variable or a log
// stores: strings stay as they are, null becomes "null" and other values are
// encoded as JSON.
func ValueToString(value goja.Value) string {
	if value == nil || goja.IsUndefined(value) {
		return ""
	}
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

// ReadStringMap reads the own properties of a JavaScript object as strings. A
// missing value yields an empty map.
func ReadStringMap(runtime *goja.Runtime, value goja.Value) map[string]string {
	result := map[string]string{}
	if IsAbsent(value) {
		return result
	}

	object := value.ToObject(runtime)
	for _, name := range object.Keys() {
		result[name] = object.Get(name).String()
	}
	return result
}

// StringMapToObject copies values into a new JavaScript object.
func StringMapToObject(runtime *goja.Runtime, values map[string]string) *goja.Object {
	object := runtime.NewObject()
	for name, value := range values {
		_ = object.Set(name, value)
	}
	return object
}
