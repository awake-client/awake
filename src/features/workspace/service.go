package workspace

// Service exposes workspace operations to the application layer.
type Service struct{}

// NewService returns a workspace service.
func NewService() *Service {
	return &Service{}
}

// Create initializes a workspace and returns its configuration.
func (service *Service) Create(rootPath string, options CreateOptions) (Configuration, error) {
	return Create(rootPath, options)
}

// Open reads and validates an existing workspace.
func (service *Service) Open(rootPath string) (Configuration, error) {
	return NewRepository(rootPath).Load()
}
