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

func TestEncodeValueUsesFlowStyleForFlatEntryLists(t *testing.T) {
	type entry struct {
		Name    string `yaml:"name"`
		Value   string `yaml:"value"`
		Enabled bool   `yaml:"enabled"`
	}
	type sample struct {
		Parameters []entry `yaml:"parameters"`
	}

	output, err := EncodeValue(sample{Parameters: []entry{
		{Name: "page", Value: "1", Enabled: true},
		{Name: "query", Value: "", Enabled: false},
	}})
	if err != nil {
		t.Fatalf("encode value: %v", err)
	}

	expected := "parameters:\n  - {name: page, value: \"1\", enabled: true}\n  - {name: query, value: \"\", enabled: false}\n"
	if string(output) != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}
