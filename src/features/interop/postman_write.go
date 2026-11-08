package interop

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/awake-client/awake/src/domain"
)

// WritePostman renders collections as a Postman collection, version 2.1. Each
// Awake collection becomes a folder of the document.
func WritePostman(collections []Collection) ([]byte, error) {
	variables := map[string]string{}
	items := make([]any, 0, len(collections))

	for _, collection := range collections {
		addPostmanVariables(variables, collection.Variables)

		folder := map[string]any{
			"name": collection.Name,
			"item": postmanItems(collection.Requests, collection.Folders),
		}
		if collection.Documentation != "" {
			folder["description"] = collection.Documentation
		}
		items = append(items, folder)
	}

	name := "Awake export"
	if len(collections) == 1 && strings.TrimSpace(collections[0].Name) != "" {
		name = collections[0].Name
	}

	return encodePostman(name, items, variables)
}

// WritePostmanCollection renders one collection as a Postman collection, with
// its requests at the root instead of in a folder of the same name.
func WritePostmanCollection(collection Collection) ([]byte, error) {
	variables := map[string]string{}
	addPostmanVariables(variables, collection.Variables)

	name := strings.TrimSpace(collection.Name)
	if name == "" {
		name = "Awake export"
	}

	return encodePostman(name, postmanItems(collection.Requests, collection.Folders), variables)
}

func addPostmanVariables(variables map[string]string, collectionVariables map[string]string) {
	for name, value := range collectionVariables {
		if _, exists := variables[name]; !exists {
			variables[name] = value
		}
	}
}

func encodePostman(name string, items []any, variables map[string]string) ([]byte, error) {
	postmanVariables := make([]any, 0, len(variables))
	for name, value := range variables {
		postmanVariables = append(postmanVariables, map[string]any{
			"key":   name,
			"value": value,
			"type":  "string",
		})
	}

	document := map[string]any{
		"info": map[string]any{
			"name":   name,
			"schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
		"item":     items,
		"variable": postmanVariables,
	}

	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(encoded, '\n'), nil
}

// WritePostmanEnvironment renders an environment as a Postman environment
// document. Secret variables keep their name but not their value, as Postman
// does when it exports them.
func WritePostmanEnvironment(environment Environment) ([]byte, error) {
	secrets := make(map[string]bool, len(environment.SecretVariables))
	for _, name := range environment.SecretVariables {
		secrets[name] = true
	}

	names := make([]string, 0, len(environment.Variables))
	for name := range environment.Variables {
		if !secrets[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	values := make([]any, 0, len(names)+len(environment.SecretVariables))
	for _, name := range names {
		values = append(values, map[string]any{
			"key":     name,
			"value":   environment.Variables[name],
			"type":    "default",
			"enabled": true,
		})
	}
	for _, name := range environment.SecretVariables {
		values = append(values, map[string]any{
			"key":     name,
			"value":   "",
			"type":    "secret",
			"enabled": true,
		})
	}

	document := map[string]any{
		"name":                    environment.Name,
		"values":                  values,
		"_postman_variable_scope": "environment",
	}

	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(encoded, '\n'), nil
}

func postmanItems(requests []Request, folders []Folder) []any {
	items := make([]any, 0, len(requests)+len(folders))

	for _, folder := range folders {
		item := map[string]any{
			"name": folder.Name,
			"item": postmanItems(folder.Requests, folder.Folders),
		}
		if folder.Documentation != "" {
			item["description"] = folder.Documentation
		}
		items = append(items, item)
	}

	for _, request := range requests {
		items = append(items, map[string]any{
			"name":    request.Name,
			"request": postmanRequestValue(request.Request),
		})
	}

	return items
}

func postmanRequestValue(request domain.Request) map[string]any {
	value := map[string]any{
		"method": request.Method,
		"header": postmanHeaders(request.Headers),
		"url":    postmanURLValueFromRequest(request),
	}

	if request.Documentation != "" {
		value["description"] = request.Documentation
	}
	if body := postmanBodyFromRequest(request.Body); body != nil {
		value["body"] = body
	}
	if auth := postmanAuthFromRequest(request.Authentication); auth != nil {
		value["auth"] = auth
	}

	return value
}

func postmanHeaders(headers []domain.Header) []any {
	values := make([]any, 0, len(headers))
	for _, header := range headers {
		if header.Name == "" {
			continue
		}
		values = append(values, map[string]any{
			"key":      header.Name,
			"value":    header.Value,
			"type":     "text",
			"disabled": !header.Enabled,
		})
	}
	return values
}

func postmanURLValueFromRequest(request domain.Request) map[string]any {
	query := make([]any, 0, len(request.Parameters))
	for _, parameter := range request.Parameters {
		if parameter.Name == "" {
			continue
		}
		query = append(query, map[string]any{
			"key":      parameter.Name,
			"value":    parameter.Value,
			"disabled": !parameter.Enabled,
		})
	}

	result := map[string]any{
		"raw":   request.URL,
		"query": query,
	}
	if len(request.PathParameters) > 0 {
		variables := make([]any, 0, len(request.PathParameters))
		for _, parameter := range request.PathParameters {
			variables = append(variables, map[string]any{"key": parameter.Name, "value": parameter.Value})
		}
		result["variable"] = variables
	}
	return result
}

func postmanBodyFromRequest(body *domain.Body) map[string]any {
	if body == nil || body.Type == domain.BodyNone {
		return nil
	}

	switch body.Type {
	case domain.BodyJSON, domain.BodyText, domain.BodyXML:
		language := "text"
		if body.Type == domain.BodyJSON {
			language = "json"
		}
		if body.Type == domain.BodyXML {
			language = "xml"
		}

		return map[string]any{
			"mode": "raw",
			"raw":  body.Content,
			"options": map[string]any{
				"raw": map[string]any{"language": language},
			},
		}
	case domain.BodyFormURLEncoded:
		return map[string]any{
			"mode":       "urlencoded",
			"urlencoded": postmanFormEntries(body.Fields),
		}
	case domain.BodyMultipart:
		return map[string]any{
			"mode":     "formdata",
			"formdata": postmanFormEntries(body.Fields),
		}
	case domain.BodyBinary:
		return map[string]any{
			"mode": "file",
			"file": map[string]any{"src": body.Content},
		}
	default:
		return nil
	}
}

func postmanFormEntries(fields []domain.Parameter) []any {
	entries := make([]any, 0, len(fields))
	for _, field := range fields {
		entry := map[string]any{
			"key":   field.Name,
			"value": field.Value,
			"type":  "text",
		}
		if !field.Enabled {
			entry["disabled"] = true
		}
		entries = append(entries, entry)
	}
	return entries
}

func postmanAuthFromRequest(auth *domain.Authentication) map[string]any {
	if auth == nil || auth.Type == domain.AuthenticationInherit {
		return nil
	}

	switch auth.Type {
	case domain.AuthenticationNone:
		return map[string]any{"type": "noauth"}
	case domain.AuthenticationBearer, domain.AuthenticationOAuth2:
		return map[string]any{
			"type":   "bearer",
			"bearer": []any{postmanAuthValue("token", auth.Token)},
		}
	case domain.AuthenticationBasic:
		return map[string]any{
			"type": "basic",
			"basic": []any{
				postmanAuthValue("username", auth.Username),
				postmanAuthValue("password", auth.Password),
			},
		}
	case domain.AuthenticationAPIKey:
		location := auth.Location
		if location != "query" {
			location = "header"
		}
		return map[string]any{
			"type": "apikey",
			"apikey": []any{
				postmanAuthValue("key", auth.Key),
				postmanAuthValue("value", auth.Value),
				postmanAuthValue("in", location),
			},
		}
	default:
		return nil
	}
}

func postmanAuthValue(key string, value string) map[string]any {
	return map[string]any{"key": key, "value": value, "type": "string"}
}
