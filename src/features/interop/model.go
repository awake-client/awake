// Package interop converts workspace collections to and from OpenAPI, Postman
// and Bruno documents.
package interop

import (
	"errors"

	"github.com/clementherve/awake/src/domain"
)

// Format identifies an interchange document format.
type Format string

const (
	// FormatOpenAPI is an OpenAPI 3 document, in JSON or YAML.
	FormatOpenAPI Format = "openapi"
	// FormatPostman is a Postman collection, version 2.0 or 2.1.
	FormatPostman Format = "postman"
	// FormatBruno is a Bruno collection directory, in .bru or OpenCollection
	// YAML form.
	FormatBruno Format = "bruno"
)

var (
	// ErrUnsupportedFormat is returned when no reader recognises a document.
	ErrUnsupportedFormat = errors.New("interop: unsupported format")
	// ErrEmptyCollection is returned when a document contains no request.
	ErrEmptyCollection = errors.New("interop: the document contains no request")
)

// Collection is a parsed collection, ready to be written to a workspace.
type Collection struct {
	Name           string
	Documentation  string
	Variables      map[string]string
	Authentication *domain.Authentication
	Requests       []Request
	Folders        []Folder
	Environments   []Environment
	// Warnings lists what the reader skipped, such as requests of a kind
	// Awake does not support.
	Warnings []string
}

// Environment is a set of variables shipped with a collection, imported as
// an environment of that collection.
type Environment struct {
	Name      string
	Variables map[string]string
	// SecretVariables are declared without a value: the source keeps secret
	// values out of its files.
	SecretVariables []string
}

// Folder groups requests inside a collection.
type Folder struct {
	Name          string
	Documentation string
	Requests      []Request
	Folders       []Folder
}

// Request pairs a request with the name it should be stored under.
type Request struct {
	Name    string
	Request domain.Request
}

// ImportResult summarizes what an import wrote to the workspace.
type ImportResult struct {
	Collections  int
	Requests     int
	Environments int
	Warnings     []string
}

// ExportResult lists the files and directories an export wrote.
type ExportResult struct {
	Paths []string
}
