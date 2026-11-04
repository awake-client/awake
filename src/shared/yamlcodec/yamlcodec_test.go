package yamlcodec

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodeRoundTripPreservesFormatting(t *testing.T) {
	paths := []string{
		"testdata/workspace.yaml",
		"testdata/environment.yaml",
		"testdata/request.yaml",
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			input := readTestFile(t, path)

			document, err := Parse(input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			output, err := document.Encode()
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			if string(output) != string(input) {
				t.Errorf("round trip changed the document\n--- input ---\n%s\n--- output ---\n%s", input, output)
			}
		})
	}
}

func TestEncodePreservesCommentsAndKeyOrderAfterChange(t *testing.T) {
	input := readTestFile(t, "testdata/workspace.yaml")

	document, err := Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	name, err := document.Find("name")
	if err != nil {
		t.Fatalf("find name: %v", err)
	}
	name.Value = "Platform API v2"

	output, err := document.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	encoded := string(output)
	if !strings.Contains(encoded, "# Workspace configuration") {
		t.Errorf("comment was lost:\n%s", encoded)
	}
	if !strings.Contains(encoded, "name: Platform API v2") {
		t.Errorf("value was not updated:\n%s", encoded)
	}

	versionPosition := strings.Index(encoded, "version:")
	idPosition := strings.Index(encoded, "id:")
	namePosition := strings.Index(encoded, "name:")
	if !(versionPosition < idPosition && idPosition < namePosition) {
		t.Errorf("key order changed:\n%s", encoded)
	}
}

func TestParseRejectsMultipleDocuments(t *testing.T) {
	_, err := Parse([]byte("name: one\n---\nname: two\n"))
	if !errors.Is(err, ErrMultipleDocuments) {
		t.Fatalf("expected ErrMultipleDocuments, got %v", err)
	}
}

func TestParseRejectsEmptyDocument(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Fatal("expected an error for an empty document")
	}
}

func TestFindReturnsNestedValue(t *testing.T) {
	document, err := Parse(readTestFile(t, "testdata/workspace.yaml"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	value, err := document.Find("git", "branch")
	if err != nil {
		t.Fatalf("find git.branch: %v", err)
	}
	if value.Value != "main" {
		t.Errorf("expected main, got %q", value.Value)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}
