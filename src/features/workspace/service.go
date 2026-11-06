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

// UpdateGit replaces the git synchronization settings stored in awake.yaml.
func (service *Service) UpdateGit(rootPath string, git GitConfiguration) (Configuration, error) {
	repository := NewRepository(rootPath)
	configuration, err := repository.Load()
	if err != nil {
		return Configuration{}, err
	}

	configuration.Git = &git
	if err := repository.Save(configuration); err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}
