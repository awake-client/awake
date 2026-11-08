package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/files"
)

const (
	editorDirectoryName = ".vscode"
	// editorSchemaDirectory holds the JSON schemas of the workspace files,
	// next to the editor settings so they are shared with them through git.
	editorSchemaDirectory  = ".vscode/awake"
	editorSettingsFileName = "settings.json"
	// yamlSchemasSetting is read by the YAML language server, used by the
	// YAML extensions of VS Code, Cursor and others. Its associations win
	// over the public SchemaStore catalog, whose unrelated schemas would
	// otherwise claim files such as collections/*/*.yaml.
	yamlSchemasSetting = "yaml.schemas"
	jsonSchemaDraft    = "http://json-schema.org/draft-07/schema#"
	variableNameRegex  = "^[A-Za-z_][A-Za-z0-9_]*$"
)

// editorSchema is one schema written to the workspace and the files it
// describes, as YAML language server globs; "!" excludes files.
type editorSchema struct {
	fileName string
	schema   map[string]any
	globs    []string
}

// EnsureEditorSchemas writes the JSON schemas of the workspace files and
// associates them with those files in the editor settings, so editors
// validate and complete them. Files are only rewritten when they change, and
// an existing settings file that is not plain JSON is left untouched.
func EnsureEditorSchemas(rootPath string) error {
	schemas := editorSchemas()

	directory := filepath.Join(rootPath, filepath.FromSlash(editorSchemaDirectory))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create editor schema directory: %w", err)
	}
	for _, schema := range schemas {
		content, err := json.MarshalIndent(schema.schema, "", "  ")
		if err != nil {
			return fmt.Errorf("encode schema %s: %w", schema.fileName, err)
		}
		if err := writeIfChanged(filepath.Join(directory, schema.fileName), append(content, '\n')); err != nil {
			return err
		}
	}

	return associateSchemas(rootPath, schemas)
}

func associateSchemas(rootPath string, schemas []editorSchema) error {
	path := filepath.Join(rootPath, editorDirectoryName, editorSettingsFileName)
	settings := map[string]any{}

	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("read editor settings: %w", err)
	default:
		// Settings with comments or trailing commas are JSONC: rewriting
		// them would lose what the user wrote, so they are left as they are.
		if json.Unmarshal(existing, &settings) != nil {
			return nil
		}
	}

	associations, _ := settings[yamlSchemasSetting].(map[string]any)
	if associations == nil {
		associations = map[string]any{}
	}
	for _, schema := range schemas {
		associations[editorSchemaDirectory+"/"+schema.fileName] = schema.globs
	}
	settings[yamlSchemasSetting] = associations

	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode editor settings: %w", err)
	}
	return writeIfChanged(path, append(content, '\n'))
}

func writeIfChanged(path string, content []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return files.WriteAtomically(path, content)
}

// editorSchemas describes every file of a workspace. Each file matches one
// schema only, since the language server combines all matching schemas.
func editorSchemas() []editorSchema {
	return []editorSchema{
		{
			fileName: "workspace.schema.json",
			schema:   workspaceSchema(),
			globs:    []string{configurationFileName},
		},
		{
			fileName: "collection.schema.json",
			schema:   collectionSchema(),
			globs:    []string{"collections/*/collection.yaml", "local/*/collection.yaml"},
		},
		{
			fileName: "folder.schema.json",
			schema:   folderSchema(),
			globs:    []string{"collections/**/folder.yaml", "local/**/folder.yaml"},
		},
		{
			fileName: "request.schema.json",
			schema:   requestSchema(),
			globs: []string{
				"collections/**/*.yaml", "local/**/*.yaml",
				"!**/collection.yaml", "!**/folder.yaml", "!**/environments/*.yaml", "!**/*.local.yaml",
			},
		},
		{
			fileName: "request-override.schema.json",
			schema:   requestOverrideSchema(),
			globs:    []string{"collections/**/*.local.yaml", "local/**/*.local.yaml", "!**/environments/*.local.yaml"},
		},
		{
			fileName: "environment.schema.json",
			schema:   environmentSchema(true),
			globs:    []string{"environments/*.yaml", "!**/*.local.yaml"},
		},
		{
			fileName: "environment-local.schema.json",
			schema:   environmentSchema(false),
			globs:    []string{"environments/*.local.yaml"},
		},
	}
}

func document(title string, properties map[string]any, required ...string) map[string]any {
	schema := object(properties, required...)
	schema["$schema"] = jsonSchemaDraft
	schema["title"] = title
	return schema
}

func object(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func text(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

// scalar accepts unquoted numbers and booleans, which YAML decodes into the
// string fields of Awake.
func scalar(description string) map[string]any {
	return map[string]any{"type": []string{"string", "number", "boolean"}, "description": description}
}

func enumeration(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": description}
}

func variables(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"propertyNames":        map[string]any{"pattern": variableNameRegex},
		"additionalProperties": scalar("Value; use {{name}} to refer to another variable."),
	}
}

func color() map[string]any {
	return enumeration("Color shown in the interface.", append([]string{""}, domain.Colors...)...)
}

func entries(kind string) map[string]any {
	return map[string]any{
		"type": "array",
		"items": object(map[string]any{
			"name":    text(kind + " name."),
			"value":   scalar(kind + " value; may contain {{variables}}."),
			"enabled": map[string]any{"type": "boolean", "description": "Whether the " + kind + " is sent."},
		}, "name"),
	}
}

func pathParameters() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Values of the :name placeholders of the URL, in its path or as whole query values.",
		"items": object(map[string]any{
			"name":  text("Name of the :name segment."),
			"value": scalar("Value; may contain {{variables}}, and is escaped for the path."),
		}, "name"),
	}
}

func authentication() map[string]any {
	return object(map[string]any{
		"type": enumeration("How the request authenticates; inherit uses the collection's authentication.",
			string(domain.AuthenticationInherit), string(domain.AuthenticationNone), string(domain.AuthenticationBasic),
			string(domain.AuthenticationBearer), string(domain.AuthenticationAPIKey), string(domain.AuthenticationOAuth2)),
		"username":         text("Basic authentication user name."),
		"password":         text("Basic authentication password."),
		"token":            text("Bearer token, or a static OAuth 2 token."),
		"key":              text("API key name."),
		"value":            text("API key value."),
		"location":         enumeration("Where the API key is sent.", "header", "query"),
		"grantType":        enumeration("OAuth 2 grant.", string(domain.OAuth2GrantAuthorizationCode), string(domain.OAuth2GrantClientCredentials), string(domain.OAuth2GrantPassword)),
		"authorizationUrl": text("OAuth 2 authorization endpoint."),
		"tokenUrl":         text("OAuth 2 token endpoint."),
		"clientId":         text("OAuth 2 client identifier."),
		"clientSecret":     text("OAuth 2 client secret."),
		"scopes":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "OAuth 2 scopes."},
		"redirectUrl":      text("OAuth 2 redirect URL."),
		"audience":         text("OAuth 2 audience."),
	}, "type")
}

func body() map[string]any {
	return object(map[string]any{
		"type": enumeration("Body format.",
			string(domain.BodyNone), string(domain.BodyJSON), string(domain.BodyText), string(domain.BodyXML),
			string(domain.BodyFormURLEncoded), string(domain.BodyMultipart), string(domain.BodyBinary)),
		"content": text("Body content; may contain {{variables}}."),
		"fields":  entries("form field"),
	}, "type")
}

func requestSchema() map[string]any {
	methods := []string{
		domain.MethodGet, domain.MethodPost, domain.MethodPut, domain.MethodPatch,
		domain.MethodDelete, domain.MethodHead, domain.MethodOptions,
	}
	// Methods are read case-insensitively.
	for _, method := range append([]string(nil), methods...) {
		methods = append(methods, strings.ToLower(method))
	}

	return document("Awake request", map[string]any{
		"name":           text("Display name."),
		"method":         enumeration("HTTP method.", methods...),
		"url":            text("URL; may contain {{variables}}."),
		"protocol":       enumeration("Protocol; http when omitted.", string(domain.ProtocolHTTP), string(domain.ProtocolSSE), string(domain.ProtocolWebSocket)),
		"parameters":     entries("query parameter"),
		"pathParameters": pathParameters(),
		"headers":        entries("header"),
		"authentication": authentication(),
		"body":           body(),
		"scripts": object(map[string]any{
			"preRequest":   text("JavaScript run before the request is sent."),
			"postResponse": text("JavaScript run after the response is received."),
		}),
		"documentation": text("Free-form notes, in Markdown."),
	}, "name", "method", "url")
}

func requestOverrideSchema() map[string]any {
	return document("Awake request local overrides", map[string]any{
		"parameters":     entries("query parameter"),
		"pathParameters": pathParameters(),
		"body":           body(),
	})
}

func collectionSchema() map[string]any {
	return document("Awake collection", map[string]any{
		"name":           text("Display name."),
		"variables":      variables("Variables of every request of the collection."),
		"authentication": authentication(),
		"documentation":  text("Free-form notes, in Markdown."),
		"color":          color(),
	}, "name")
}

func folderSchema() map[string]any {
	return document("Awake folder", map[string]any{
		"name": text("Display name."),
	}, "name")
}

func environmentSchema(shared bool) map[string]any {
	title := "Awake environment"
	required := []string{"name"}
	if !shared {
		title = "Awake environment local overrides"
		required = nil
	}
	return document(title, map[string]any{
		"name":      text("Display name; the file name follows it."),
		"variables": variables("Variables of the environment."),
		"secretVariables": map[string]any{
			"type":        "array",
			"description": "Variables whose values are kept in the system keychain.",
			"items":       map[string]any{"type": "string", "pattern": variableNameRegex},
			"uniqueItems": true,
		},
		"color": color(),
	}, required...)
}

func workspaceSchema() map[string]any {
	return document("Awake workspace", map[string]any{
		"version":            map[string]any{"type": "integer", "description": "File format version."},
		"id":                 text("Identifier generated once, keying local data."),
		"name":               text("Display name."),
		"mode":               enumeration("Team workspaces synchronize through git.", string(ModeTeam), string(ModePersonal)),
		"defaultEnvironment": text("Environment selected when the workspace opens."),
		"variables":          variables("Variables of every request of the workspace."),
		"git": object(map[string]any{
			"remote":                  text("Git remote URL."),
			"branch":                  text("Branch to synchronize."),
			"automaticPull":           map[string]any{"type": "boolean"},
			"automaticPush":           map[string]any{"type": "boolean"},
			"synchronizationInterval": text("Interval between synchronizations, such as 5m."),
		}),
		"color": color(),
	}, "version", "id", "name", "mode")
}
