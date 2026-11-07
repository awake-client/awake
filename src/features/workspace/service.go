package workspace

import "log"

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

// Open reads and validates an existing workspace, and refreshes the editor
// schemas of its files so they follow this version of Awake.
func (service *Service) Open(rootPath string) (Configuration, error) {
	configuration, err := NewRepository(rootPath).Load()
	if err != nil {
		return Configuration{}, err
	}
	// A workspace stays usable without editor schemas, in a read-only
	// directory for instance, so a failure is only reported in the log.
	if err := EnsureEditorSchemas(rootPath); err != nil {
		log.Printf("workspace: editor schemas not updated in %s: %v", rootPath, err)
	}
	return configuration, nil
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

// UpdateColor stores the color of the workspace in awake.yaml.
func (service *Service) UpdateColor(rootPath string, color string) (Configuration, error) {
	repository := NewRepository(rootPath)
	configuration, err := repository.Load()
	if err != nil {
		return Configuration{}, err
	}

	configuration.Color = color
	if err := repository.Save(configuration); err != nil {
		return Configuration{}, err
	}
	return configuration, nil
}
