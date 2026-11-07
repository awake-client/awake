package interop

import (
	"encoding/json"

	"github.com/clementherve/awake/src/domain"
)

// WritePostman renders collections as a Postman collection, version 2.1.
func WritePostman(collections []Collection) ([]byte, error) {
	variables := map[string]string{}
	items := make([]any, 0, len(collections))

	for _, collection := range collections {
		for name, value := range collection.Variables {
			if _, exists := variables[name]; !exists {
				variables[name] = value
			}
		}

		folder := map[string]any{
			"name": collection.Name,
			"item": postmanItems(collection.Requests, collection.Folders),
		}
		if collection.Documentation != "" {
			folder["description"] = collection.Documentation
		}
		items = append(items, folder)
	}

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
			"name":   "Awake export",
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
