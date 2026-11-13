package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/awake-client/awake/src/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/text/unicode/norm"
)

const requestFields = "Fields: name, method, url (may use {{variables}}), parameters and headers " +
	"(lists of {name, value, enabled}), authentication ({type: inherit|none|basic|bearer|apiKey|oauth2, ...}), " +
	"body ({type: none|json|text|xml|form-urlencoded|multipart|binary, ...}), scripts " +
	"({preRequest, postResponse}) and documentation. Read the awake://guide resource for every field."

type writeRequestInput struct {
	Path    string         `json:"path" jsonschema:"request file to write, such as collections/billing/invoices/create.yaml"`
	Request map[string]any `json:"request" jsonschema:"the request, with the fields of its YAML file"`
}

type createFolderInput struct {
	Path string `json:"path" jsonschema:"folder to create inside an existing collection or folder, such as collections/billing/invoices"`
	Name string `json:"name,omitempty" jsonschema:"display name, the last path segment by default"`
}

type createCollectionInput struct {
	Name    string `json:"name" jsonschema:"display name of the collection"`
	Private bool   `json:"private,omitempty" jsonschema:"keep it on this computer, ignored by git"`
}

type pathOutput struct {
	Path string `json:"path"`
}

func (tools *tools) registerWrite(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "create_request",
		Description: "Create a request file. Fails when it already exists. Missing folders are created. " + requestFields,
		Annotations: &sdk.ToolAnnotations{Title: "Create a request", DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(false)},
	}, tools.createRequest)

	sdk.AddTool(server, &sdk.Tool{
		Name: "update_request",
		Description: "Replace a request with a new version. Read it with get_request first and send it back " +
			"whole: fields left out are removed. Fails when it does not exist. " + requestFields,
		Annotations: &sdk.ToolAnnotations{Title: "Update a request", DestructiveHint: boolPointer(true), IdempotentHint: true, OpenWorldHint: boolPointer(false)},
	}, tools.updateRequest)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "create_folder",
		Description: "Create a folder inside an existing collection or folder.",
		Annotations: &sdk.ToolAnnotations{Title: "Create a folder", DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(false)},
	}, tools.createFolder)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "create_collection",
		Description: "Create a collection, shared with the team by default or private to this computer.",
		Annotations: &sdk.ToolAnnotations{Title: "Create a collection", DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(false)},
	}, tools.createCollection)
}

func (tools *tools) createRequest(_ context.Context, _ *sdk.CallToolRequest, input writeRequestInput) (*sdk.CallToolResult, requestOutput, error) {
	request, err := requestFromObject(input.Request)
	if err != nil {
		return nil, requestOutput{}, err
	}
	if err := tools.workspace.CreateRequest(input.Path, request); err != nil {
		return nil, requestOutput{}, describeWriteError(input.Path, err)
	}
	return tools.savedRequest(input.Path)
}

func (tools *tools) updateRequest(_ context.Context, _ *sdk.CallToolRequest, input writeRequestInput) (*sdk.CallToolResult, requestOutput, error) {
	request, err := requestFromObject(input.Request)
	if err != nil {
		return nil, requestOutput{}, err
	}
	if err := tools.workspace.UpdateRequest(input.Path, request); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, requestOutput{}, fmt.Errorf("%s does not exist; use create_request to add it", input.Path)
		}
		return nil, requestOutput{}, describeWriteError(input.Path, err)
	}
	return tools.savedRequest(input.Path)
}

// savedRequest returns the request as written, defaults included.
func (tools *tools) savedRequest(path string) (*sdk.CallToolResult, requestOutput, error) {
	request, err := tools.workspace.LoadRequest(path)
	if err != nil {
		return nil, requestOutput{}, err
	}
	object, err := requestToObject(request)
	if err != nil {
		return nil, requestOutput{}, err
	}
	return nil, requestOutput{Path: path, Request: object}, nil
}

func (tools *tools) createFolder(_ context.Context, _ *sdk.CallToolRequest, input createFolderInput) (*sdk.CallToolResult, pathOutput, error) {
	path := strings.TrimSuffix(input.Path, "/")
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = path[strings.LastIndex(path, "/")+1:]
	}
	if err := tools.workspace.CreateFolder(path, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, pathOutput{}, fmt.Errorf("the parent of %s does not exist; create the collection or folder first", path)
		}
		return nil, pathOutput{}, describeWriteError(path, err)
	}
	return nil, pathOutput{Path: path}, nil
}

func (tools *tools) createCollection(_ context.Context, _ *sdk.CallToolRequest, input createCollectionInput) (*sdk.CallToolResult, pathOutput, error) {
	name := strings.TrimSpace(input.Name)
	slug := slugify(name)
	if slug == "" {
		return nil, pathOutput{}, errors.New("choose a collection name with at least one letter or digit")
	}
	root := "collections"
	if input.Private {
		root = "local"
	}
	path := root + "/" + slug
	if err := tools.workspace.CreateCollection(path, domain.Collection{Name: name}); err != nil {
		return nil, pathOutput{}, describeWriteError(path, err)
	}
	return nil, pathOutput{Path: path}, nil
}

func describeWriteError(path string, err error) error {
	if errors.Is(err, ErrExists) {
		return fmt.Errorf("%s already exists", path)
	}
	return describeEntryError(path, err)
}

// slugify turns a display name into a directory name with the rule of the
// interface: accents and symbols removed, lower case, words joined by dashes.
func slugify(name string) string {
	var builder strings.Builder
	dash := false
	for _, character := range strings.ToLower(norm.NFKD.String(name)) {
		switch {
		case character >= 0x300 && character <= 0x36f:
			// A combining accent, left over from its letter.
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			builder.WriteRune(character)
			dash = false
		case builder.Len() > 0 && !dash:
			builder.WriteRune('-')
			dash = true
		}
	}
	return strings.TrimSuffix(builder.String(), "-")
}
