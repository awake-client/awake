package mcp

import (
	"bytes"
	"fmt"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/yamlcodec"
	"go.yaml.in/yaml/v3"
)

// Requests cross the protocol with the same fields as the request files, so
// agents can rely on the documented file format rather than on Go names.

// requestToObject converts a request to the object of its YAML file.
func requestToObject(request domain.Request) (map[string]any, error) {
	content, err := yamlcodec.EncodeValue(request)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	object := map[string]any{}
	if err := yaml.Unmarshal(content, &object); err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}
	return object, nil
}

// requestFromObject reads a request written as the object of its YAML file.
// Unknown fields are rejected, so a typo is reported instead of ignored.
func requestFromObject(object map[string]any) (domain.Request, error) {
	content, err := yaml.Marshal(object)
	if err != nil {
		return domain.Request{}, fmt.Errorf("encode request: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	var request domain.Request
	if err := decoder.Decode(&request); err != nil {
		return domain.Request{}, fmt.Errorf("invalid request: %w", err)
	}
	return request, nil
}
