package yamlcodec

import (
	"testing"
)

func TestEncodeValueUsesCanonicalIndentation(t *testing.T) {
	type nested struct {
		Key string `yaml:"key"`
	}
	type sample struct {
		Name   string `yaml:"name"`
		Nested nested `yaml:"nested"`
	}

	output, err := EncodeValue(sample{Name: "value", Nested: nested{Key: "nested-value"}})
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}

	expected := "name: value\nnested:\n  key: nested-value\n"
	if string(output) != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}
