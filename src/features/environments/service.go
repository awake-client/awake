package environments

// Service exposes environment operations to the application layer.
type Service struct{}

// NewService returns an environment service.
func NewService() *Service {
	return &Service{}
}

// List returns the shared environments of a workspace.
func (service *Service) List(workspaceRoot string) ([]Summary, error) {
	return NewRepository(workspaceRoot).List()
}

// Load returns an environment with its local overrides applied.
func (service *Service) Load(workspaceRoot string, fileName string) (ResolvedEnvironment, error) {
	return NewRepository(workspaceRoot).Load(fileName)
}
