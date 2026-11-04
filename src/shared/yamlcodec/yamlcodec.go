// Package yamlcodec reads and writes YAML documents while preserving the
// formatting choices that make git diffs readable: comments, key order and
// scalar styles.
//
// Files written by Awake use two-space indentation, LF line endings and compact
// flow mappings for list entries. Parsing an existing file and encoding it back
// without modification must produce the same bytes.
package yamlcodec

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

// ErrMultipleDocuments is returned when a file contains more than one YAML
// document. Awake stores exactly one document per file.
var ErrMultipleDocuments = errors.New("yamlcodec: multiple documents are not supported")

// Document wraps a parsed YAML document. The underlying node tree retains
// comments and ordering, so callers can mutate values and encode the document
// back without losing formatting.
type Document struct {
	root *yaml.Node
}

// Parse decodes a single YAML document from data.
func Parse(data []byte) (*Document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse yaml document: %w", err)
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		return nil, ErrMultipleDocuments
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse trailing yaml content: %w", err)
	}

	return &Document{root: &root}, nil
}

// Encode serializes the document with the canonical formatting rules.
func (document *Document) Encode() ([]byte, error) {
	var buffer bytes.Buffer

	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)

	if err := encoder.Encode(document.root); err != nil {
		return nil, fmt.Errorf("encode yaml document: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}

	return buffer.Bytes(), nil
}

// Decode maps the document onto a Go value.
func (document *Document) Decode(target any) error {
	if err := document.root.Decode(target); err != nil {
		return fmt.Errorf("decode yaml document: %w", err)
	}
	return nil
}

// Root returns the document node. Its first content node is the mapping that
// holds the document body.
func (document *Document) Root() *yaml.Node {
	return document.root
}

// Find returns the value stored at the given key path.
func (document *Document) Find(keys ...string) (*yaml.Node, error) {
	current := document.mapping()
	if current == nil {
		return nil, errors.New("yamlcodec: document is not a mapping")
	}

	for _, key := range keys {
		value := mappingValue(current, key)
		if value == nil {
			return nil, fmt.Errorf("yamlcodec: key %q not found", key)
		}
		current = value
	}

	return current, nil
}

func (document *Document) mapping() *yaml.Node {
	if len(document.root.Content) == 0 {
		return nil
	}

	body := document.root.Content[0]
	if body.Kind != yaml.MappingNode {
		return nil
	}

	return body
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}
