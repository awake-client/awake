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
	"sort"

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

// EncodeValue serializes a Go value with the canonical formatting rules. The
// value is marshaled first, then normalized through the document encoder so
// that indentation and line endings match files written by Awake.
func EncodeValue(value any) ([]byte, error) {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal yaml value: %w", err)
	}

	document, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("normalize yaml value: %w", err)
	}

	applyCompactSequenceEntries(document.root)

	return document.Encode()
}

func applyCompactSequenceEntries(node *yaml.Node) {
	if node.Kind == yaml.SequenceNode && sequenceHoldsFlatMappings(node) {
		for _, item := range node.Content {
			item.Style = yaml.FlowStyle
		}
	}

	for _, child := range node.Content {
		applyCompactSequenceEntries(child)
	}
}

func sequenceHoldsFlatMappings(node *yaml.Node) bool {
	if len(node.Content) == 0 {
		return false
	}

	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return false
		}
		for index := 1; index < len(item.Content); index += 2 {
			if item.Content[index].Kind != yaml.ScalarNode {
				return false
			}
		}
	}

	return true
}

// Decode maps the document onto a Go value.
func (document *Document) Decode(target any) error {
	if err := document.root.Decode(target); err != nil {
		return fmt.Errorf("decode yaml document: %w", err)
	}
	return nil
}

// SetScalar sets or adds a string value under key in a mapping, keeping the
// other keys and their comments.
func SetScalar(mapping *yaml.Node, key string, value string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1].Kind = yaml.ScalarNode
			mapping.Content[index+1].Tag = "!!str"
			mapping.Content[index+1].Value = value
			mapping.Content[index+1].Content = nil
			return
		}
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// SetStringMap sets or adds a string map under key in a mapping, sorted by
// key. An empty map removes the key.
func SetStringMap(mapping *yaml.Node, key string, values map[string]string) {
	if len(values) == 0 {
		RemoveMappingValue(mapping, key)
		return
	}

	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, name := range keys {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: values[name]},
		)
	}

	SetMappingValue(mapping, key, node)
}

// SetStringSequence sets or adds a string sequence under key in a mapping. An
// empty sequence removes the key.
func SetStringSequence(mapping *yaml.Node, key string, values []string) {
	if len(values) == 0 {
		RemoveMappingValue(mapping, key)
		return
	}

	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: value,
		})
	}

	SetMappingValue(mapping, key, node)
}

// SetMappingValue sets or adds a node under key in a mapping.
func SetMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = value
			return
		}
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

// RemoveMappingValue removes the value under key from a mapping.
func RemoveMappingValue(mapping *yaml.Node, key string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
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
