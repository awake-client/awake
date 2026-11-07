package interop

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/clementherve/awake/src/domain"
)

// ReadOpenAPI parses an OpenAPI 3 or Swagger 2 document. Both YAML and JSON are
// accepted because YAML is a superset of JSON.
func ReadOpenAPI(content []byte) ([]Collection, error) {
	document, err := decodeDocument(content)
	if err != nil {
		return nil, err
	}

	openAPIVersion := stringValue(document["openapi"])
	swaggerVersion := stringValue(document["swagger"])
	if openAPIVersion == "" && swaggerVersion == "" {
		return nil, ErrUnsupportedFormat
	}

	info := mapValue(document["info"])
	name := stringValue(info["title"])
	if strings.TrimSpace(name) == "" {
		name = "Imported API"
	}

	baseURL := "{{baseUrl}}"
	variables := map[string]string{}

	if openAPIVersion != "" {
		servers := sliceValue(document["servers"])
		if len(servers) > 0 {
			server := mapValue(servers[0])
			if serverURL := stringValue(server["url"]); serverURL != "" && serverURL != "{baseUrl}" && serverURL != "{{baseUrl}}" {
				variables["baseUrl"] = serverURL
			}
		}
	} else {
		host := stringValue(document["host"])
		basePath := stringValue(document["basePath"])
		scheme := "https"
		schemes := sliceValue(document["schemes"])
		if len(schemes) > 0 {
			scheme = stringValue(schemes[0])
		}
		if host != "" {
			variables["baseUrl"] = scheme + "://" + host + basePath
		} else if basePath != "" {
			variables["baseUrl"] = basePath
		}
	}

	collection := Collection{
		Name:          name,
		Documentation: stringValue(info["description"]),
		Variables:     variables,
	}

	paths := mapValue(document["paths"])
	folders := map[string]*Folder{}
	folderOrder := make([]string, 0)

	for _, requestPath := range sortedKeys(paths) {
		pathItem := mapValue(paths[requestPath])

		for _, method := range openAPIMethods {
			operation := mapValue(pathItem[method])
			if operation == nil {
				continue
			}

			request := openAPIRequest(method, requestPath, pathItem, operation, baseURL)
			tag := firstTag(operation)
			if tag == "" {
				collection.Requests = append(collection.Requests, request)
				continue
			}

			if _, found := folders[tag]; !found {
				folders[tag] = &Folder{Name: tag}
				folderOrder = append(folderOrder, tag)
			}
			folders[tag].Requests = append(folders[tag].Requests, request)
		}
	}

	for _, tag := range folderOrder {
		collection.Folders = append(collection.Folders, *folders[tag])
	}

	if len(collection.Requests) == 0 && len(collection.Folders) == 0 {
		return nil, ErrEmptyCollection
	}

	return []Collection{collection}, nil
}

var openAPIMethods = []string{
	"get", "post", "put", "patch", "delete", "head", "options", "trace",
}

func openAPIRequest(method string, requestPath string, pathItem map[string]any, operation map[string]any, baseURL string) Request {
	name := stringValue(operation["summary"])
	if name == "" {
		name = stringValue(operation["operationId"])
	}
	if name == "" {
		name = strings.ToUpper(method) + " " + requestPath
	}

	request := domain.Request{
		Name:           name,
		Method:         strings.ToUpper(method),
		URL:            baseURL + pathTemplate(requestPath),
		Documentation:  documentText(operation),
		Authentication: &domain.Authentication{Type: domain.AuthenticationInherit},
		Body:           &domain.Body{Type: domain.BodyNone},
	}

	seenParameters := map[string]bool{}
	for _, parameters := range [][]any{sliceValue(pathItem["parameters"]), sliceValue(operation["parameters"])} {
		for _, rawParameter := range parameters {
			parameter := mapValue(rawParameter)
			parameterName := stringValue(parameter["name"])
			if parameterName == "" || seenParameters[parameterName] {
				continue
			}
			seenParameters[parameterName] = true

			value := parameterValue(parameter)
			switch stringValue(parameter["in"]) {
			case "path":
				request.PathParameters = append(request.PathParameters, domain.PathParameter{Name: parameterName, Value: value})
			case "query":
				request.Parameters = append(request.Parameters, domain.Parameter{
					Name:    parameterName,
					Value:   value,
					Enabled: !boolValue(parameter["required"]) || value != "",
				})
			case "header":
				request.Headers = append(request.Headers, domain.Header{
					Name:    parameterName,
					Value:   value,
					Enabled: true,
				})
			}
		}
	}

	request.Body = openAPIBody(operation["requestBody"])

	return Request{Name: name, Request: request}
}

// pathTemplate turns the {name} segments of an OpenAPI path into the :name
// segments Awake fills from its path parameters.
func pathTemplate(requestPath string) string {
	return openAPIPathParameter.ReplaceAllString(requestPath, ":$1")
}

var openAPIPathParameter = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func documentText(operation map[string]any) string {
	description := stringValue(operation["description"])
	if description != "" {
		return description
	}
	return stringValue(operation["summary"])
}

func openAPIBody(rawBody any) *domain.Body {
	requestBody := mapValue(rawBody)
	if requestBody == nil {
		return &domain.Body{Type: domain.BodyNone}
	}

	content := mapValue(requestBody["content"])
	mediaType, media := preferredMediaType(content)
	if media == nil {
		return &domain.Body{Type: domain.BodyNone}
	}

	bodyType := bodyTypeForMediaType(mediaType)
	example := media["example"]
	if example == nil {
		schema := mapValue(media["schema"])
		example = schema["example"]
	}
	if example == nil {
		example = mapValue(media["examples"])["default"]
		if nested := mapValue(example); nested != nil {
			example = nested["value"]
		}
	}

	body := &domain.Body{Type: bodyType}
	if body.IsForm() {
		body.Fields = formFields(example, mapValue(media["schema"]))
		return body
	}
	body.Content = encodeValue(example)
	return body
}

// formFields turns an example object into form fields, or lists the
// properties of the schema with empty values when there is no example.
func formFields(example any, schema map[string]any) []domain.Parameter {
	values := mapValue(example)
	names := make([]string, 0)
	if values != nil {
		for name := range values {
			names = append(names, name)
		}
	} else {
		for name := range mapValue(schema["properties"]) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	fields := make([]domain.Parameter, 0, len(names))
	for _, name := range names {
		value := ""
		if values != nil {
			value = scalarString(values[name])
		}
		fields = append(fields, domain.Parameter{Name: name, Value: value, Enabled: true})
	}
	return fields
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return encodeValue(typed)
	}
}

func preferredMediaType(content map[string]any) (string, map[string]any) {
	if content == nil {
		return "", nil
	}

	for _, candidate := range []string{
		"application/json", "application/xml", "text/plain",
		"application/x-www-form-urlencoded", "application/octet-stream",
	} {
		if media, found := content[candidate]; found {
			return candidate, mapValue(media)
		}
	}

	for mediaType, media := range content {
		if strings.HasPrefix(mediaType, "application/") || strings.HasPrefix(mediaType, "text/") {
			return mediaType, mapValue(media)
		}
	}

	return "", nil
}

func bodyTypeForMediaType(mediaType string) domain.BodyType {
	switch {
	case strings.Contains(mediaType, "json"):
		return domain.BodyJSON
	case strings.Contains(mediaType, "xml"):
		return domain.BodyXML
	case strings.Contains(mediaType, "x-www-form-urlencoded"):
		return domain.BodyFormURLEncoded
	case strings.Contains(mediaType, "multipart"):
		return domain.BodyMultipart
	case strings.HasPrefix(mediaType, "text/"):
		return domain.BodyText
	default:
		return domain.BodyBinary
	}
}

func parameterValue(parameter map[string]any) string {
	if example, found := parameter["example"]; found {
		return encodeValue(example)
	}

	schema := mapValue(parameter["schema"])
	if schema != nil {
		for _, key := range []string{"example", "default", "enum"} {
			if value, found := schema[key]; found {
				return firstValue(value)
			}
		}
	}

	return ""
}

func firstTag(operation map[string]any) string {
	tags := sliceValue(operation["tags"])
	if len(tags) == 0 {
		return ""
	}
	return stringValue(tags[0])
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstValue(value any) string {
	switch typed := value.(type) {
	case []any:
		if len(typed) > 0 {
			return encodeValue(typed[0])
		}
		return ""
	default:
		return encodeValue(value)
	}
}

func mapValue(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func sliceValue(value any) []any {
	if value == nil {
		return nil
	}
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func boolValue(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

func encodeValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case []any, map[string]any:
		encoded, err := encodeJSON(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return encoded
	default:
		return fmt.Sprint(typed)
	}
}
