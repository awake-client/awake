package collections

import "github.com/clementherve/awake/src/domain"

// Service exposes collection operations to the application layer.
type Service struct{}

// NewService returns a collection service.
func NewService() *Service {
	return &Service{}
}

// List returns the direct children of a directory. An empty path lists the
// collections of the workspace.
func (service *Service) List(workspaceRoot string, relativePath string) ([]Entry, error) {
	return NewRepository(workspaceRoot).List(relativePath)
}

// LoadRequest returns a request with defaults applied.
func (service *Service) LoadRequest(workspaceRoot string, relativePath string) (domain.Request, error) {
	return NewRepository(workspaceRoot).LoadRequest(relativePath)
}

// LoadCollection returns collection metadata, or ErrEntryNotFound when the
// directory has no collection file.
func (service *Service) LoadCollection(workspaceRoot string, relativePath string) (domain.Collection, error) {
	return NewRepository(workspaceRoot).LoadCollection(relativePath)
}

// CreateRequest writes a new request file.
func (service *Service) CreateRequest(workspaceRoot string, relativePath string, request domain.Request) error {
	return NewRepository(workspaceRoot).CreateRequest(relativePath, request)
}

// UpdateRequest replaces the content of an existing request file.
func (service *Service) UpdateRequest(workspaceRoot string, relativePath string, request domain.Request) error {
	return NewRepository(workspaceRoot).UpdateRequest(relativePath, request)
}

// CreateCollection creates a collection directory with its metadata file.
func (service *Service) CreateCollection(workspaceRoot string, relativePath string, collection domain.Collection) error {
	return NewRepository(workspaceRoot).CreateCollection(relativePath, collection)
}

// CreateFolder creates a folder inside an existing collection or folder.
func (service *Service) CreateFolder(workspaceRoot string, relativePath string) error {
	return NewRepository(workspaceRoot).CreateFolder(relativePath)
}

// Delete removes a request, or a folder or collection with its content.
func (service *Service) Delete(workspaceRoot string, relativePath string) error {
	return NewRepository(workspaceRoot).Delete(relativePath)
}

// SearchRequests returns requests matching every word of the query.
func (service *Service) SearchRequests(workspaceRoot string, query string, limit int) ([]Entry, error) {
	return NewRepository(workspaceRoot).SearchRequests(query, limit)
}
