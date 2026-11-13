// Package mcp exposes the requests of one workspace to coding agents through
// the Model Context Protocol. The server runs over stdio, launched by the
// agent itself with `awake mcp`, so it never listens on the network.
//
// The package only declares the ports it needs; the application layer wires
// them to the collections, HTTP client, environments and history features.
package mcp

import (
	"errors"

	"github.com/awake-client/awake/src/domain"
)

var (
	// ErrNotFound is returned by the ports when an entry does not exist.
	ErrNotFound = errors.New("mcp: not found")
	// ErrExists is returned by the ports when an entry already exists.
	ErrExists = errors.New("mcp: already exists")
)

// Entry is a collection, folder or request of the workspace tree.
type Entry struct {
	Path   string
	Name   string
	Kind   string
	Method string
	// HasChildren reports a collection or folder that holds entries.
	HasChildren bool
}

// Entry kinds.
const (
	KindCollection = "collection"
	KindFolder     = "folder"
	KindRequest    = "request"
)

// InheritedAuthentication is the authentication a request inherits and the
// folder or collection defining it.
type InheritedAuthentication struct {
	Type string
	From string
}

// Environment summarizes an environment file.
type Environment struct {
	FileName string
	Name     string
	// Scope is empty for a global environment, else the collection or folder.
	Scope   string
	Private bool
}

// Workspace reads and writes the entries of the workspace served.
type Workspace interface {
	// List returns the children of a collection or folder, or the
	// collections when path is empty.
	List(path string) ([]Entry, error)
	SearchRequests(query string, limit int) ([]Entry, error)
	LoadRequest(path string) (domain.Request, error)
	InheritedAuthentication(path string) (InheritedAuthentication, error)
	CreateRequest(path string, request domain.Request) error
	UpdateRequest(path string, request domain.Request) error
	CreateFolder(path string, name string) error
	CreateCollection(path string, collection domain.Collection) error
	Environments() ([]Environment, error)
	// DefaultEnvironment is the environment used when a call names none.
	DefaultEnvironment() string
	// Guide documents the workspace file formats for agents.
	Guide() string
}

// Response is a sent request's response, its body limited to a preview.
type Response struct {
	StatusCode   int
	Status       string
	Headers      map[string][]string
	ContentType  string
	Body         string
	Binary       bool
	Truncated    bool
	Size         int64
	DurationMs   int64
	PreRequest   domain.ScriptOutcome
	PostResponse domain.ScriptOutcome
}

// Variable is a variable visible to a request.
type Variable struct {
	Name   string
	Value  string
	Source string
	Secret bool
}

// Sender sends requests exactly as the Send button does.
type Sender interface {
	// Send sends a request and returns at most maxBodyBytes of its body.
	Send(path string, environment string, maxBodyBytes int) (Response, error)
	Variables(path string, environment string) ([]Variable, error)
}

// History reads the requests sent from the workspace.
type History interface {
	List(requestPath string, limit int) ([]domain.HistoryEntry, error)
	Entry(id int64) (domain.HistoryEntry, error)
}
