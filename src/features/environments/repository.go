package environments

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Repository reads environment files from a workspace.
type Repository struct {
	workspaceRoot string
}

// NewRepository returns a repository rooted at the workspace directory.
func NewRepository(workspaceRoot string) *Repository {
	return &Repository{workspaceRoot: workspaceRoot}
}

// DirectoryPath returns the environments directory of the workspace.
func (repository *Repository) DirectoryPath() string {
	return filepath.Join(repository.workspaceRoot, directoryName)
}

// SharedPath returns the path of a shared environment file.
func (repository *Repository) SharedPath(fileName string) string {
	return filepath.Join(repository.DirectoryPath(), fileName+fileExtension)
}

// LocalPath returns the path of a local override file.
func (repository *Repository) LocalPath(fileName string) string {
	return filepath.Join(repository.DirectoryPath(), fileName+localSuffix)
}

// List returns the shared environments, ignoring local override files.
func (repository *Repository) List() ([]Summary, error) {
	entries, err := os.ReadDir(repository.DirectoryPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Summary{}, nil
		}
		return nil, fmt.Errorf("read environments directory: %w", err)
	}

	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !strings.HasSuffix(fileName, fileExtension) || strings.HasSuffix(fileName, localSuffix) {
			continue
		}

		baseName := strings.TrimSuffix(fileName, fileExtension)

		environment, err := repository.readShared(baseName)
		if err != nil {
			return nil, fmt.Errorf("read environment %s: %w", baseName, err)
		}

		summaries = append(summaries, Summary{
			Name:              environment.Name,
			FileName:          baseName,
			HasLocalOverrides: fileExists(repository.LocalPath(baseName)),
		})
	}

	sort.Slice(summaries, func(first int, second int) bool {
		return summaries[first].Name < summaries[second].Name
	})

	return summaries, nil
}

// Load returns a shared environment merged with its local overrides.
func (repository *Repository) Load(fileName string) (ResolvedEnvironment, error) {
	if err := validateFileName(fileName); err != nil {
		return ResolvedEnvironment{}, err
	}

	shared, err := repository.readShared(fileName)
	if err != nil {
		return ResolvedEnvironment{}, err
	}

	resolved := ResolvedEnvironment{
		Name:   shared.Name,
		Shared: shared,
	}

	local, err := repository.readLocal(fileName)
	if err != nil {
		return ResolvedEnvironment{}, err
	}
	resolved.Local = local

	return resolved, nil
}

func (repository *Repository) readShared(fileName string) (Environment, error) {
	environment, err := readEnvironment(repository.SharedPath(fileName), ErrEnvironmentNotFound)
	if err != nil {
		return Environment{}, err
	}
	if err := environment.validate(); err != nil {
		return Environment{}, err
	}
	return environment, nil
}

func (repository *Repository) readLocal(fileName string) (*Environment, error) {
	environment, err := readEnvironment(repository.LocalPath(fileName), nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	if environment.Name == "" {
		environment.Name = fileName
	}
	if err := environment.validate(); err != nil {
		return nil, err
	}

	return &environment, nil
}

func readEnvironment(path string, missingError error) (Environment, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && missingError != nil {
			return Environment{}, missingError
		}
		return Environment{}, fmt.Errorf("read %s: %w", path, err)
	}

	var environment Environment
	if err := yaml.Unmarshal(content, &environment); err != nil {
		return Environment{}, fmt.Errorf("%w: %v", ErrInvalidEnvironment, err)
	}

	return environment, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
