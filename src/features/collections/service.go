package collections

import "github.com/awake-client/awake/src/domain"

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

// LoadRequestOverride returns the parts of a request replaced on this
// machine, empty when there are none.
func (service *Service) LoadRequestOverride(workspaceRoot string, relativePath string) (domain.RequestOverride, error) {
	return NewRepository(workspaceRoot).LoadRequestOverride(relativePath)
}

// SaveRequestOverride writes the parts of a request replaced on this machine,
// or removes them when the override is empty.
func (service *Service) SaveRequestOverride(workspaceRoot string, relativePath string, override domain.RequestOverride) error {
	return NewRepository(workspaceRoot).SaveRequestOverride(relativePath, override)
}

// LoadCollection returns collection metadata, or ErrEntryNotFound when the
// directory has no collection file.
func (service *Service) LoadCollection(workspaceRoot string, relativePath string) (domain.Collection, error) {
	return NewRepository(workspaceRoot).LoadCollection(relativePath)
}

// LoadInheritedAuthentication returns the authentication an inheriting request
// receives from its closest folder or collection, and where it comes from.
func (service *Service) LoadInheritedAuthentication(workspaceRoot string, requestPath string) (InheritedAuthentication, error) {
	return NewRepository(workspaceRoot).ResolveInheritedAuthentication(requestPath)
}

// LoadInheritedAuthenticationForScope returns the authentication a folder
// inherits from the folders and collection above it.
func (service *Service) LoadInheritedAuthenticationForScope(workspaceRoot string, scopePath string) (InheritedAuthentication, error) {
	return NewRepository(workspaceRoot).ResolveInheritedAuthenticationForScope(scopePath)
}

// CreateRequest writes a new request file.
func (service *Service) CreateRequest(workspaceRoot string, relativePath string, request domain.Request) error {
	return NewRepository(workspaceRoot).CreateRequest(relativePath, request)
}

// UpdateRequest replaces the content of an existing request file.
func (service *Service) UpdateRequest(workspaceRoot string, relativePath string, request domain.Request) error {
	return NewRepository(workspaceRoot).UpdateRequest(relativePath, request)
}

// UpdateCollection replaces the metadata of an existing collection.
func (service *Service) UpdateCollection(workspaceRoot string, relativePath string, collection domain.Collection) error {
	return NewRepository(workspaceRoot).UpdateCollection(relativePath, collection)
}

// CreateCollection creates a collection directory with its metadata file.
func (service *Service) CreateCollection(workspaceRoot string, relativePath string, collection domain.Collection) error {
	return NewRepository(workspaceRoot).CreateCollection(relativePath, collection)
}

// CreateFolder creates a folder inside an existing collection or folder.
func (service *Service) CreateFolder(workspaceRoot string, relativePath string, name string) error {
	return NewRepository(workspaceRoot).CreateFolder(relativePath, name)
}

// Delete removes a request, or a folder or collection with its content.
func (service *Service) Delete(workspaceRoot string, relativePath string) error {
	return NewRepository(workspaceRoot).Delete(relativePath)
}

// Move moves a request or a folder into another collection or folder and
// returns its new path.
func (service *Service) Move(workspaceRoot string, relativePath string, targetDirectory string) (string, error) {
	return NewRepository(workspaceRoot).Move(relativePath, targetDirectory)
}

// SearchRequests returns requests matching every word of the query.
func (service *Service) SearchRequests(workspaceRoot string, query string, limit int) ([]Entry, error) {
	return NewRepository(workspaceRoot).SearchRequests(query, limit)
}
