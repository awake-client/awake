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
	directoryName = "collections"
	// privateDirectoryName holds collections that stay on this computer. The
	// workspace ignores it in git.
	privateDirectoryName = "local"
	collectionFileName   = "collection.yaml"
	fileExtension        = ".yaml"
	// folderKeepFileName keeps an empty folder in git, which ignores empty
	// directories.
	folderKeepFileName = ".gitkeep"
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

// isRoot reports whether a cleaned path is one of the two collection roots.
func isRoot(cleaned string) bool {
	return cleaned == directoryName || cleaned == privateDirectoryName
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
			if relativePath == "" || isRoot(path.Clean(relativePath)) {
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

// SearchRequests returns the requests whose name, path or method match every
// word of the query. An empty query returns the first requests of the
// workspace, which lets the command palette fill itself when it opens.
func (repository *Repository) SearchRequests(query string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}

	terms := strings.Fields(strings.ToLower(query))
	result := make([]Entry, 0, limit)

	walk := func(filePath string, entry os.DirEntry, walkError error) error {
		if walkError != nil {
			if errors.Is(walkError, os.ErrNotExist) {
				return nil
			}
			return walkError
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), fileExtension) || entry.Name() == collectionFileName {
			return nil
		}

		relativePath, err := filepath.Rel(repository.workspaceRoot, filePath)
		if err != nil {
			return err
		}

		name, method, err := readRequestSummary(filePath, entry.Name())
		if err != nil {
			return fmt.Errorf("read request %s: %w", relativePath, err)
		}

		requestPath := filepath.ToSlash(relativePath)
		candidate := Entry{
			Path:   requestPath,
			Name:   name,
			Kind:   KindRequest,
			Method: method,
		}
		if matchesQuery(candidate, terms) {
			result = append(result, candidate)
		}

		return nil
	}

	for _, root := range []string{directoryName, privateDirectoryName} {
		err := filepath.WalkDir(filepath.Join(repository.workspaceRoot, root), walk)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("search requests: %w", err)
		}
	}

	sort.Slice(result, func(first int, second int) bool {
		return entryLess(result[first], result[second])
	})

	if len(result) > limit {
		result = result[:limit]
	}

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

// LoadCollection reads and validates a collection metadata file. A collection
// directory without metadata is valid and returns ErrEntryNotFound so callers
// can fall back to the directory name.
func (repository *Repository) LoadCollection(relativePath string) (domain.Collection, error) {
	if strings.HasSuffix(relativePath, fileExtension) {
		return domain.Collection{}, ErrInvalidPath
	}

	directory, err := repository.resolve(relativePath)
	if err != nil {
		return domain.Collection{}, err
	}

	content, err := os.ReadFile(filepath.Join(directory, collectionFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Collection{}, ErrEntryNotFound
		}
		return domain.Collection{}, fmt.Errorf("read collection %s: %w", relativePath, err)
	}

	var collection domain.Collection
	if err := yaml.Unmarshal(content, &collection); err != nil {
		return domain.Collection{}, fmt.Errorf("%w: %v", domain.ErrInvalidCollection, err)
	}
	if err := collection.Validate(); err != nil {
		return domain.Collection{}, err
	}

	return collection, nil
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

// UpdateRequest replaces the content of an existing request file.
func (repository *Repository) UpdateRequest(relativePath string, request domain.Request) error {
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

	if _, err := os.Stat(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrEntryNotFound
		}
		return fmt.Errorf("inspect request %s: %w", relativePath, err)
	}

	content, err := yamlcodec.EncodeValue(request)
	if err != nil {
		return fmt.Errorf("encode request %s: %w", relativePath, err)
	}

	return files.WriteAtomically(filePath, content)
}

// CreateCollection creates a collection directory with its metadata file.
func (repository *Repository) CreateCollection(relativePath string, collection domain.Collection) error {
	if relativePath == "" || isRoot(path.Clean(relativePath)) {
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

// Delete removes a request file, or a folder or collection with everything it
// contains. The collections directory itself cannot be deleted.
func (repository *Repository) Delete(relativePath string) error {
	cleaned := path.Clean(relativePath)
	if cleaned == "." || isRoot(cleaned) {
		return ErrInvalidPath
	}

	target, err := repository.resolve(cleaned)
	if err != nil {
		return err
	}

	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrEntryNotFound
		}
		return fmt.Errorf("inspect %s: %w", relativePath, err)
	}

	if !info.IsDir() {
		if !isRequestPath(cleaned) {
			return ErrInvalidPath
		}
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("delete request %s: %w", relativePath, err)
		}
		return nil
	}

	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("delete folder %s: %w", relativePath, err)
	}
	return nil
}

// CreateFolder creates a folder inside an existing collection or folder.
// Collections themselves are created with CreateCollection.
func (repository *Repository) CreateFolder(relativePath string) error {
	cleaned := path.Clean(relativePath)
	if strings.Count(cleaned, "/") < 2 || isRequestPath(cleaned) {
		return ErrInvalidPath
	}

	directory, err := repository.resolve(cleaned)
	if err != nil {
		return err
	}

	parent := filepath.Dir(directory)
	if info, err := os.Stat(parent); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrEntryNotFound
		}
		return fmt.Errorf("inspect folder parent %s: %w", relativePath, err)
	} else if !info.IsDir() {
		return ErrInvalidPath
	}

	if _, err := os.Stat(directory); err == nil {
		return ErrEntryExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect folder %s: %w", relativePath, err)
	}

	if err := os.Mkdir(directory, 0o755); err != nil {
		return fmt.Errorf("create folder %s: %w", relativePath, err)
	}

	return files.WriteAtomically(filepath.Join(directory, folderKeepFileName), nil)
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
	root := strings.SplitN(cleaned, "/", 2)[0]
	if !isRoot(root) {
		return "", ErrInvalidPath
	}

	rootPath := filepath.Join(repository.workspaceRoot, root)
	filePath := filepath.Join(repository.workspaceRoot, filepath.FromSlash(cleaned))
	relativeToCollections, err := filepath.Rel(rootPath, filePath)
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

func matchesQuery(entry Entry, terms []string) bool {
	if len(terms) == 0 {
		return true
	}

	haystack := strings.ToLower(entry.Name + " " + entry.Path + " " + entry.Method)
	for _, term := range terms {
		if !strings.Contains(haystack, term) {
			return false
		}
	}

	return true
}
