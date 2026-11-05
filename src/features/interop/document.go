package interop

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// decodeDocument reads a JSON or YAML document into a generic map.
func decodeDocument(content []byte) (map[string]any, error) {
	document := map[string]any{}
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}
	if len(document) == 0 {
		return nil, ErrUnsupportedFormat
	}
	return document, nil
}

// encodeJSON writes compact JSON, which fits request bodies and examples.
func encodeJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// jsonExample turns a request body into an OpenAPI example value. Valid JSON is
// kept structured so generated documents stay readable.
func jsonExample(content string) any {
	var decoded any
	if err := json.Unmarshal([]byte(content), &decoded); err == nil {
		return decoded
	}
	return content
}

// yamlMarshal is a thin seam so writers share the same encoder.
func yamlMarshal(value any) ([]byte, error) {
	return yaml.Marshal(value)
}

// detectDocumentFormat guesses a format from a path and, when needed, from the
// beginning of the document.
func detectDocumentFormat(path string, content []byte) Format {
	lowerPath := strings.ToLower(path)

	switch {
	case strings.HasSuffix(lowerPath, ".bru"):
		return FormatBruno
	case strings.HasSuffix(lowerPath, ".yaml"), strings.HasSuffix(lowerPath, ".yml"):
		return FormatOpenAPI
	case strings.HasSuffix(lowerPath, ".json"):
		if strings.Contains(string(content), `"item"`) && strings.Contains(string(content), `"info"`) {
			return FormatPostman
		}
		return FormatOpenAPI
	}

	if strings.Contains(string(content), `"openapi"`) {
		return FormatOpenAPI
	}
	if strings.Contains(string(content), `"swagger"`) {
		return FormatOpenAPI
	}
	if strings.Contains(string(content), `"item"`) {
		return FormatPostman
	}

	return ""
}
