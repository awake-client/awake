package environments

import (
	"os"
	"path/filepath"
	"sort"
)

// Service exposes environment operations to the application layer. Every
// method takes a scope: an empty string for the global environments of the
// workspace, or a collection path for the environments of that collection.
type Service struct{}

// NewService returns an environment service.
func NewService() *Service {
	return &Service{}
}

// ScopedSummary is an environment together with the scope defining it.
type ScopedSummary struct {
	Summary
	Scope string
}

// List returns the environments of a scope.
func (service *Service) List(workspaceRoot string, scope string) ([]Summary, error) {
	repository, err := NewScopedRepository(workspaceRoot, scope)
	if err != nil {
		return nil, err
	}
	return repository.List()
}

// ListAll returns the global environments followed by the environments of
// every collection, so the interface can offer each name once.
func (service *Service) ListAll(workspaceRoot string) ([]ScopedSummary, error) {
	global, err := NewRepository(workspaceRoot).List()
	if err != nil {
		return nil, err
	}
	result := make([]ScopedSummary, 0, len(global))
	for _, summary := range global {
		result = append(result, ScopedSummary{Summary: summary})
	}

	for _, root := range []string{sharedCollectionsRoot, privateCollectionsRoot} {
		entries, err := os.ReadDir(filepath.Join(workspaceRoot, root))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		sort.Slice(entries, func(first int, second int) bool { return entries[first].Name() < entries[second].Name() })
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			scope := root + "/" + entry.Name()
			repository, err := NewScopedRepository(workspaceRoot, scope)
			if err != nil {
				continue
			}
			summaries, err := repository.List()
			if err != nil {
				return nil, err
			}
			for _, summary := range summaries {
				result = append(result, ScopedSummary{Summary: summary, Scope: scope})
			}
		}
	}
	return result, nil
}

// Load returns an environment with its local overrides applied.
func (service *Service) Load(workspaceRoot string, scope string, fileName string) (ResolvedEnvironment, error) {
	repository, err := NewScopedRepository(workspaceRoot, scope)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	return repository.Load(fileName)
}

// Save writes a shared environment file.
func (service *Service) Save(workspaceRoot string, scope string, fileName string, environment Environment) error {
	repository, err := NewScopedRepository(workspaceRoot, scope)
	if err != nil {
		return err
	}
	return repository.Save(fileName, environment)
}

// Delete removes a shared environment and its local overrides.
func (service *Service) Delete(workspaceRoot string, scope string, fileName string) error {
	repository, err := NewScopedRepository(workspaceRoot, scope)
	if err != nil {
		return err
	}
	return repository.Delete(fileName)
}
