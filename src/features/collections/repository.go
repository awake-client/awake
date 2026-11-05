// Package collections indexes the collection tree stored in the workspace.
package collections

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/shared/files"
	"github.com/clementherve/awake/src/shared/yamlcodec"
)

const (
	directoryName      = "collections"
	collectionFileName = "collection.yaml"
	fileExtension      = ".yaml"
)

var (
	// ErrEntryNotFound is returned when a collection or request does not exist.
	ErrEntryNotFound = errors.New("collections: entry not found")
	// ErrEntryExists is returned when creating an entry that already exists.
	ErrEntryExists = errors.New("collections: entry already exists")
	// ErrInvalidPath is returned when a path escapes the collections directory.
	ErrInvalidPath = errors.New("collections: invalid path")
)

// Kind describes the type of a tree entry.
type Kind string

const (
	// KindCollection is a top-level collection directory.
	KindCollection Kind = "collection"
	// KindDirectory is a folder inside a collection.
	KindDirectory Kind = "directory"
	// KindRequest is a request file.
	KindRequest Kind = "request"
)

// Entry is one node of the collection tree. Paths are workspace relative and
// use forward slashes on every platform.
type Entry struct {
	Path        string
	Name        string
	Kind        Kind
	Method      string
	HasChildren bool
}

// Repository reads the collection tree from a workspace.
type Repository struct {
	workspaceRoot string
}

// NewRepository returns a repository rooted at the workspace directory.
func NewRepository(workspaceRoot string) *Repository {
	return &Repository{workspaceRoot: workspaceRoot}
}

// CollectionsPath returns the absolute path of the collections directory.
func (repository *Repository) CollectionsPath() string {
	return filepath.Join(repository.workspaceRoot, directoryName)
}

// List returns the direct children of a directory. An empty path lists the
// collections themselves.
func (repository *Repository) List(relativePath string) ([]Entry, error) {
	directory, err := repository.resolve(relativePath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("read collections directory: %w", err)
	}

	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		entryPath := joinRelative(relativePath, entry.Name())

		if entry.IsDir() {
			kind := KindDirectory
			if relativePath == "" {
				kind = KindCollection
			}

			name := entry.Name()
			if kind == KindCollection {
				collectionName, err := readCollectionName(filepath.Join(directory, entry.Name()), name)
				if err != nil {
					return nil, fmt.Errorf("read collection %s: %w", entryPath, err)
				}
				name = collectionName
			}

			hasChildren, err := hasChildren(filepath.Join(directory, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", entryPath, err)
			}

			result = append(result, Entry{
				Path:        entryPath,
				Name:        name,
				Kind:        kind,
				HasChildren: hasChildren,
			})
			continue
		}

		if !strings.HasSuffix(entry.Name(), fileExtension) || entry.Name() == collectionFileName {
			continue
		}

		requestName, method, err := readRequestSummary(filepath.Join(directory, entry.Name()), entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read request %s: %w", entryPath, err)
		}

		result = append(result, Entry{
			Path:   entryPath,
			Name:   requestName,
			Kind:   KindRequest,
			Method: method,
		})
	}

	sort.Slice(result, func(first int, second int) bool {
		return entryLess(result[first], result[second])
	})

	return result, nil
}

// LoadRequest reads and validates a request file.
func (repository *Repository) LoadRequest(relativePath string) (domain.Request, error) {
	if !isRequestPath(relativePath) {
		return domain.Request{}, ErrInvalidPath
	}

	filePath, err := repository.resolve(relativePath)
	if err != nil {
		return domain.Request{}, err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Request{}, ErrEntryNotFound
		}
		return domain.Request{}, fmt.Errorf("read request %s: %w", relativePath, err)
	}

	var request domain.Request
	if err := yaml.Unmarshal(content, &request); err != nil {
		return domain.Request{}, fmt.Errorf("%w: %v", domain.ErrInvalidRequest, err)
	}

	request.ApplyDefaults()
	if err := request.Validate(); err != nil {
		return domain.Request{}, err
	}

	return request, nil
}

// CreateRequest writes a new request file and creates missing parent
// directories.
func (repository *Repository) CreateRequest(relativePath string, request domain.Request) error {
	if !isRequestPath(relativePath) {
		return ErrInvalidPath
	}

	request.ApplyDefaults()
	if err := request.Validate(); err != nil {
		return err
	}

	filePath, err := repository.resolve(relativePath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(filePath); err == nil {
		return ErrEntryExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect request %s: %w", relativePath, err)
	}

	content, err := yamlcodec.EncodeValue(request)
	if err != nil {
		return fmt.Errorf("encode request %s: %w", relativePath, err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("create request directory: %w", err)
	}

	return files.WriteAtomically(filePath, content)
}

// CreateCollection creates a collection directory with its metadata file.
func (repository *Repository) CreateCollection(relativePath string, collection domain.Collection) error {
	if relativePath == "" || relativePath == directoryName {
		return ErrInvalidPath
	}
	if err := collection.Validate(); err != nil {
		return err
	}

	directory, err := repository.resolve(relativePath)
	if err != nil {
		return err
	}

	configurationPath := filepath.Join(directory, collectionFileName)
	if _, err := os.Stat(configurationPath); err == nil {
		return ErrEntryExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect collection %s: %w", relativePath, err)
	}

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create collection directory: %w", err)
	}

	content, err := yamlcodec.EncodeValue(collection)
	if err != nil {
		return fmt.Errorf("encode collection %s: %w", relativePath, err)
	}

	return files.WriteAtomically(configurationPath, content)
}

func (repository *Repository) resolve(relativePath string) (string, error) {
	collectionsPath := repository.CollectionsPath()
	if relativePath == "" {
		return collectionsPath, nil
	}

	cleaned := path.Clean(relativePath)
	if cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrInvalidPath
	}
	if cleaned != directoryName && !strings.HasPrefix(cleaned, directoryName+"/") {
		return "", ErrInvalidPath
	}

	filePath := filepath.Join(repository.workspaceRoot, filepath.FromSlash(cleaned))
	relativeToCollections, err := filepath.Rel(collectionsPath, filePath)
	if err != nil {
		return "", ErrInvalidPath
	}
	if relativeToCollections == ".." || strings.HasPrefix(relativeToCollections, ".."+string(filepath.Separator)) {
		return "", ErrInvalidPath
	}

	return filePath, nil
}

func joinRelative(parent string, name string) string {
	if parent == "" {
		return path.Join(directoryName, name)
	}
	return path.Join(parent, name)
}

func entryLess(first Entry, second Entry) bool {
	firstIsFolder := first.Kind != KindRequest
	secondIsFolder := second.Kind != KindRequest
	if firstIsFolder != secondIsFolder {
		return firstIsFolder
	}

	firstName := strings.ToLower(first.Name)
	secondName := strings.ToLower(second.Name)
	if firstName != secondName {
		return firstName < secondName
	}

	return first.Path < second.Path
}

func readCollectionName(directory string, fallback string) (string, error) {
	content, err := os.ReadFile(filepath.Join(directory, collectionFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fallback, nil
		}
		return "", err
	}

	var collection domain.Collection
	if err := yaml.Unmarshal(content, &collection); err != nil {
		return "", fmt.Errorf("%w: %v", domain.ErrInvalidCollection, err)
	}
	if err := collection.Validate(); err != nil {
		return "", err
	}

	return collection.Name, nil
}

func readRequestSummary(filePath string, fallbackName string) (string, string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", err
	}

	var summary struct {
		Name   string `yaml:"name"`
		Method string `yaml:"method"`
	}
	if err := yaml.Unmarshal(content, &summary); err != nil {
		return "", "", fmt.Errorf("%w: %v", domain.ErrInvalidRequest, err)
	}

	name := summary.Name
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(fallbackName, fileExtension)
	}

	return name, strings.ToUpper(strings.TrimSpace(summary.Method)), nil
}

func hasChildren(directory string) (bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			return true, nil
		}
		if strings.HasSuffix(entry.Name(), fileExtension) && entry.Name() != collectionFileName {
			return true, nil
		}
	}

	return false, nil
}

func isRequestPath(relativePath string) bool {
	return strings.HasSuffix(relativePath, fileExtension) &&
		path.Base(relativePath) != collectionFileName
}
