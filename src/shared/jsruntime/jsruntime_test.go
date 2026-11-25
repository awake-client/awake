package jsruntime

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestDeadlineInterruptsALoopingScript(t *testing.T) {
	runtime := goja.New()
	deadline := NewDeadline(runtime, 20*time.Millisecond, "too slow")
	defer deadline.Stop()

	_, err := runtime.RunString(`for (;;) {}`)

	if err == nil || !strings.Contains(err.Error(), "too slow") {
		t.Fatalf("expected the interruption reason, got %v", err)
	}
}

func TestDeadlinePauseKeepsTheTimeLeft(t *testing.T) {
	runtime := goja.New()
	deadline := NewDeadline(runtime, 50*time.Millisecond, "too slow")
	defer deadline.Stop()

	deadline.Pause()
	time.Sleep(80 * time.Millisecond)
	deadline.Resume()

	if _, err := runtime.RunString(`1 + 1`); err != nil {
		t.Fatalf("a paused deadline must not interrupt the runtime: %v", err)
	}
}

func TestValueToString(t *testing.T) {
	runtime := goja.New()
	cases := map[string]string{
		`"text"`:          "text",
		`null`:            "null",
		`undefined`:       "",
		`42`:              "42",
		`({ answer: 1 })`: `{"answer":1}`,
		`[1, "two"]`:      `[1,"two"]`,
	}

	for expression, expected := range cases {
		value, err := runtime.RunString(expression)
		if err != nil {
			t.Fatalf("%s: %v", expression, err)
		}
		if actual := ValueToString(value); actual != expected {
			t.Errorf("%s: expected %q, got %q", expression, expected, actual)
		}
	}
}

func TestInstallConsolePrefixesWarningsAndErrors(t *testing.T) {
	runtime := goja.New()
	lines := []string{}
	InstallConsole(runtime, func(line string) { lines = append(lines, line) })

	if _, err := runtime.RunString(`console.log("a", 1); console.warn("b"); console.error({ c: true })`); err != nil {
		t.Fatal(err)
	}

	expected := []string{"a 1", "warning: b", `error: {"c":true}`}
	if strings.Join(lines, "|") != strings.Join(expected, "|") {
		t.Errorf("unexpected lines: %q", lines)
	}
}

func TestReadStringMapAcceptsAMissingValue(t *testing.T) {
	runtime := goja.New()
	if values := ReadStringMap(runtime, goja.Undefined()); len(values) != 0 {
		t.Errorf("expected an empty map, got %v", values)
	}

	object := StringMapToObject(runtime, map[string]string{"name": "ada"})
	if values := ReadStringMap(runtime, object); values["name"] != "ada" {
		t.Errorf("unexpected values: %v", values)
	}
}
