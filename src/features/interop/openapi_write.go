package interop

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/clementherve/awake/src/domain"
)

// WriteOpenAPI renders collections as an OpenAPI 3.1 YAML document. Requests
// grouped in Awake collections become tagged operations. A single collection
// names the document after itself.
func WriteOpenAPI(collections []Collection) ([]byte, error) {
	title := "Awake export"
	if len(collections) == 1 && strings.TrimSpace(collections[0].Name) != "" {
		title = collections[0].Name
	}

	document := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   title,
			"version": "1.0.0",
		},
		"servers": []any{
			map[string]any{"url": "{baseUrl}"},
		},
		"paths": map[string]any{},
	}

	paths := document["paths"].(map[string]any)
	reserved := map[string]map[string]bool{}

	for _, collection := range collections {
		for _, request := range flattenRequests(collection) {
			writeOpenAPIOperation(paths, reserved, collection.Name, request)
		}
	}

	encoded, err := encodeYAML(document)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// WriteOpenAPICollection renders one collection as an OpenAPI 3.1 document.
func WriteOpenAPICollection(collection Collection) ([]byte, error) {
	return WriteOpenAPI([]Collection{collection})
}

func writeOpenAPIOperation(paths map[string]any, reserved map[string]map[string]bool, tag string, request Request) {
	pathValue, pathParameters := openAPIPath(request.Request.URL)
	pathValue = reservePath(paths, reserved, pathValue, strings.ToLower(request.Request.Method), tag)

	pathItem, found := paths[pathValue].(map[string]any)
	if !found {
		pathItem = map[string]any{}
		paths[pathValue] = pathItem
	}

	operation := map[string]any{
		"summary":     request.Name,
		"operationId": operationID(tag, request),
	}
	if request.Request.Documentation != "" {
		operation["description"] = request.Request.Documentation
	}
	if tag != "" {
		operation["tags"] = []any{tag}
	}

	parameters := make([]any, 0, len(request.Request.Parameters)+len(request.Request.Headers)+len(pathParameters))

	for _, name := range pathParameters {
		parameters = append(parameters, map[string]any{
			"name":     name,
			"in":       "path",
			"required": true,
			"schema":   map[string]any{"type": "string"},
		})
	}

	for _, parameter := range request.Request.Parameters {
		if !parameter.Enabled {
			continue
		}
		parameters = append(parameters, map[string]any{
			"name":   parameter.Name,
			"in":     "query",
			"schema": map[string]any{"type": "string", "default": parameter.Value},
		})
	}

	for _, header := range request.Request.Headers {
		if !header.Enabled {
			continue
		}
		parameters = append(parameters, map[string]any{
			"name":   header.Name,
			"in":     "header",
			"schema": map[string]any{"type": "string", "default": header.Value},
		})
	}

	if len(parameters) > 0 {
		operation["parameters"] = parameters
	}

	if body := openAPIRequestBody(request.Request.Body); body != nil {
		operation["requestBody"] = body
	}

	pathItem[strings.ToLower(request.Request.Method)] = operation
}

func reservePath(paths map[string]any, reserved map[string]map[string]bool, pathValue string, method string, tag string) string {
	methods, found := reserved[pathValue]
	if !found {
		reserved[pathValue] = map[string]bool{method: true}
		return pathValue
	}
	if !methods[method] {
		methods[method] = true
		return pathValue
	}

	prefix := slugify(tag)
	if prefix == "" {
		prefix = "collection"
	}

	candidate := "/" + prefix + pathValue
	for {
		methods, found := reserved[candidate]
		if !found {
			reserved[candidate] = map[string]bool{method: true}
			return candidate
		}
		if !methods[method] {
			methods[method] = true
			return candidate
		}
		candidate += "-2"
	}
}

func openAPIPath(rawURL string) (string, []string) {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.SplitN(rawURL, "?", 2)[0]

	pathValue := ""
	switch {
	case strings.Contains(rawURL, "://"):
		parsed, err := url.Parse(rawURL)
		if err == nil {
			pathValue = parsed.Path
		}
	case strings.HasPrefix(rawURL, "{{"):
		if index := strings.Index(rawURL, "/"); index >= 0 {
			pathValue = rawURL[index:]
		}
	case strings.HasPrefix(rawURL, "/"):
		pathValue = rawURL
	default:
		pathValue = "/" + rawURL
	}

	if pathValue == "" {
		pathValue = "/"
	}

	parameters := domain.PathParameterNames(pathValue)
	pathValue, _ = domain.ReplacePathParameters(pathValue, func(placeholder domain.URLPlaceholder) (string, error) {
		return "{" + placeholder.Name + "}", nil
	})

	for {
		start := strings.Index(pathValue, "{{")
		if start < 0 {
			break
		}
		end := strings.Index(pathValue[start:], "}}")
		if end < 0 {
			break
		}
		end += start

		name := pathValue[start+2 : end]
		parameters = append(parameters, name)
		pathValue = pathValue[:start] + "{" + name + "}" + pathValue[end+2:]
	}

	return pathValue, parameters
}

func operationID(tag string, request Request) string {
	identifier := slugify(request.Name)
	if identifier == "" {
		identifier = slugify(request.Request.Method + "-" + request.Request.URL)
	}
	if tag == "" {
		return identifier
	}
	return slugify(tag) + "-" + identifier
}

func openAPIRequestBody(body *domain.Body) map[string]any {
	if body == nil || body.Type == domain.BodyNone {
		return nil
	}

	mediaType := ""
	switch body.Type {
	case domain.BodyJSON:
		mediaType = "application/json"
	case domain.BodyXML:
		mediaType = "application/xml"
	case domain.BodyText:
		mediaType = "text/plain"
	case domain.BodyFormURLEncoded:
		mediaType = "application/x-www-form-urlencoded"
	case domain.BodyMultipart:
		mediaType = "multipart/form-data"
	case domain.BodyBinary:
		mediaType = "application/octet-stream"
	default:
		return nil
	}

	media := map[string]any{}
	if body.IsForm() && len(body.Fields) > 0 {
		example := map[string]any{}
		properties := map[string]any{}
		for _, field := range body.Fields {
			if !field.Enabled {
				continue
			}
			example[field.Name] = field.Value
			properties[field.Name] = map[string]any{"type": "string"}
		}
		media["schema"] = map[string]any{"type": "object", "properties": properties}
		media["example"] = example
	} else if body.Content != "" {
		switch body.Type {
		case domain.BodyJSON:
			media["example"] = jsonExample(body.Content)
		case domain.BodyBinary:
			media["schema"] = map[string]any{"type": "string", "format": "binary"}
		default:
			media["example"] = body.Content
		}
	}

	return map[string]any{
		"content": map[string]any{mediaType: media},
	}
}

func flattenRequests(collection Collection) []Request {
	requests := make([]Request, 0, len(collection.Requests))
	requests = append(requests, collection.Requests...)

	for _, folder := range collection.Folders {
		requests = append(requests, flattenFolderRequests(folder)...)
	}

	return requests
}

func flattenFolderRequests(folder Folder) []Request {
	requests := make([]Request, 0, len(folder.Requests))
	requests = append(requests, folder.Requests...)

	for _, child := range folder.Folders {
		requests = append(requests, flattenFolderRequests(child)...)
	}

	return requests
}

func encodeYAML(value any) ([]byte, error) {
	encoded, err := yamlMarshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode document: %w", err)
	}
	return encoded, nil
}
