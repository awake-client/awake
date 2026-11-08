package interop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/clementherve/awake/src/domain"
)

// WriteBrunoDirectory writes each collection as a Bruno collection directory
// under destination. It returns the files it wrote.
func WriteBrunoDirectory(destination string, collections []Collection) ([]string, error) {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return nil, fmt.Errorf("create bruno directory: %w", err)
	}

	written := make([]string, 0)
	usedCollections := map[string]int{}

	for _, collection := range collections {
		collectionName := uniqueName(usedCollections, slugify(collection.Name), "collection")
		collectionDirectory := filepath.Join(destination, collectionName)
		if err := os.MkdirAll(collectionDirectory, 0o755); err != nil {
			return nil, fmt.Errorf("create collection directory: %w", err)
		}

		manifestPath := filepath.Join(collectionDirectory, brunoManifestFileName)
		if err := os.WriteFile(manifestPath, []byte(brunoManifestFile(collection.Name)), 0o644); err != nil {
			return nil, fmt.Errorf("write bruno manifest: %w", err)
		}
		written = append(written, manifestPath)

		configurationPath := filepath.Join(collectionDirectory, brunoCollectionFileName)
		if err := os.WriteFile(configurationPath, []byte(collectionFile(collection)), 0o644); err != nil {
			return nil, fmt.Errorf("write collection file: %w", err)
		}
		written = append(written, configurationPath)

		paths, err := writeBrunoFolder(collectionDirectory, collection.Requests, collection.Folders)
		if err != nil {
			return nil, err
		}
		written = append(written, paths...)
	}

	return written, nil
}

func writeBrunoFolder(directory string, requests []Request, folders []Folder) ([]string, error) {
	written := make([]string, 0)
	usedFiles := map[string]int{}

	for _, request := range requests {
		fileName := uniqueName(usedFiles, slugify(request.Name), "request") + brunoExtension
		filePath := filepath.Join(directory, fileName)
		if err := os.WriteFile(filePath, []byte(requestFile(request.Request)), 0o644); err != nil {
			return nil, fmt.Errorf("write request file: %w", err)
		}
		written = append(written, filePath)
	}

	usedFolders := map[string]int{}
	for index, folder := range folders {
		folderName := uniqueName(usedFolders, slugify(folder.Name), "folder")
		folderDirectory := filepath.Join(directory, folderName)
		if err := os.MkdirAll(folderDirectory, 0o755); err != nil {
			return nil, fmt.Errorf("create folder directory: %w", err)
		}

		folderFilePath := filepath.Join(folderDirectory, brunoFolderFileName)
		if err := os.WriteFile(folderFilePath, []byte(folderFile(folder.Name, index+1)), 0o644); err != nil {
			return nil, fmt.Errorf("write folder file: %w", err)
		}
		written = append(written, folderFilePath)

		paths, err := writeBrunoFolder(folderDirectory, folder.Requests, folder.Folders)
		if err != nil {
			return nil, err
		}
		written = append(written, paths...)
	}

	return written, nil
}

// brunoManifestFile is the bruno.json Bruno reads to recognise a directory as
// a collection. Bruno requires a name and the collection type, and refuses a
// directory without this manifest.
func brunoManifestFile(name string) string {
	manifest := struct {
		Version string   `json:"version"`
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Ignore  []string `json:"ignore"`
	}{
		Version: "1",
		Name:    name,
		Type:    "collection",
		Ignore:  []string{"node_modules", ".git"},
	}

	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Sprintf(
			`{"version": "1", "name": %q, "type": "collection", "ignore": ["node_modules", ".git"]}`+"\n",
			name,
		)
	}
	return string(content) + "\n"
}

func folderFile(name string, sequence int) string {
	return fmt.Sprintf("meta {\n  name: %s\n  seq: %d\n}\n", name, sequence)
}

func collectionFile(collection Collection) string {
	var builder strings.Builder

	builder.WriteString("meta {\n")
	builder.WriteString("  name: " + collection.Name + "\n")
	builder.WriteString("}\n")

	if len(collection.Variables) > 0 {
		builder.WriteString("\nvars:collection {\n")
		for name, value := range collection.Variables {
			builder.WriteString("  " + name + ": " + value + "\n")
		}
		builder.WriteString("}\n")
	}

	if collection.Documentation != "" {
		builder.WriteString("\ndocs {\n")
		builder.WriteString(indent(collection.Documentation))
		builder.WriteString("\n}\n")
	}

	if block := authBlock(collection.Authentication); block != "" {
		builder.WriteString("\n")
		builder.WriteString(block)
	}

	return builder.String()
}

func requestFile(request domain.Request) string {
	var builder strings.Builder

	builder.WriteString("meta {\n")
	builder.WriteString("  name: " + request.Name + "\n")
	builder.WriteString("  type: http\n")
	builder.WriteString("}\n")

	method := strings.ToLower(request.Method)
	if method == "" {
		method = "get"
	}

	builder.WriteString("\n" + method + " {\n")
	builder.WriteString("  url: " + request.URL + "\n")
	builder.WriteString("  body: " + string(bodyType(request.Body)) + "\n")
	builder.WriteString("  auth: " + string(authType(request.Authentication)) + "\n")
	builder.WriteString("}\n")

	if len(request.PathParameters) > 0 {
		builder.WriteString("\nparams:path {\n")
		for _, parameter := range request.PathParameters {
			builder.WriteString(brunoEntryLine(true, parameter.Name, parameter.Value))
		}
		builder.WriteString("}\n")
	}

	if len(request.Parameters) > 0 {
		builder.WriteString("\nparams:query {\n")
		for _, parameter := range request.Parameters {
			builder.WriteString(brunoEntryLine(parameter.Enabled, parameter.Name, parameter.Value))
		}
		builder.WriteString("}\n")
	}

	if len(request.Headers) > 0 {
		builder.WriteString("\nheaders {\n")
		for _, header := range request.Headers {
			builder.WriteString(brunoEntryLine(header.Enabled, header.Name, header.Value))
		}
		builder.WriteString("}\n")
	}

	if request.Body != nil && request.Body.Type != domain.BodyNone {
		builder.WriteString("\nbody:" + string(request.Body.Type) + " {\n")
		if request.Body.IsForm() {
			for _, field := range request.Body.Fields {
				builder.WriteString(brunoEntryLine(field.Enabled, field.Name, field.Value))
			}
		} else {
			builder.WriteString(request.Body.Content)
			if request.Body.Content != "" && !strings.HasSuffix(request.Body.Content, "\n") {
				builder.WriteString("\n")
			}
		}
		builder.WriteString("}\n")
	}

	if block := authBlock(request.Authentication); block != "" {
		builder.WriteString("\n")
		builder.WriteString(block)
	}

	if request.Documentation != "" {
		builder.WriteString("\ndocs {\n")
		builder.WriteString(indent(request.Documentation))
		builder.WriteString("\n}\n")
	}

	return builder.String()
}

func brunoEntryLine(enabled bool, name string, value string) string {
	prefix := "  "
	if !enabled {
		prefix = "  ~"
	}
	return prefix + name + ": " + value + "\n"
}

func authBlock(authentication *domain.Authentication) string {
	if authentication == nil {
		return ""
	}

	switch authentication.Type {
	case domain.AuthenticationBearer, domain.AuthenticationOAuth2:
		return "auth:bearer {\n  token: " + authentication.Token + "\n}\n"
	case domain.AuthenticationBasic:
		return "auth:basic {\n  username: " + authentication.Username +
			"\n  password: " + authentication.Password + "\n}\n"
	case domain.AuthenticationAPIKey:
		location := authentication.Location
		if location != "query" {
			location = "header"
		}
		return "auth:apikey {\n  key: " + authentication.Key +
			"\n  value: " + authentication.Value + "\n  in: " + location + "\n}\n"
	case domain.AuthenticationNone:
		return "auth:none {\n}\n"
	default:
		return ""
	}
}

func authType(authentication *domain.Authentication) domain.AuthenticationType {
	if authentication == nil {
		return domain.AuthenticationInherit
	}
	return authentication.Type
}

func bodyType(body *domain.Body) domain.BodyType {
	if body == nil {
		return domain.BodyNone
	}
	return body.Type
}

func indent(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = "  " + line
	}
	return strings.Join(lines, "\n")
}
