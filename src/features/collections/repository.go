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

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/files"
	"github.com/awake-client/awake/src/shared/yamlcodec"
)

const (
	directoryName = "collections"
	// privateDirectoryName holds collections that stay on this computer. The
	// workspace ignores it in git.
	privateDirectoryName = "local"
	collectionFileName   = "collection.yaml"
	// localOverrideSuffix ends the git ignored file holding the parts of a
	// request replaced on this machine only.
	localOverrideSuffix = ".local.yaml"
	fileExtension       = ".yaml"
	// folderFileName stores the display name of a folder. It also keeps an
	// empty folder in git, which ignores empty directories.
	folderFileName = "folder.yaml"
)

var (
	// ErrEntryNotFound is returned when a collection or request does not exist.
	ErrEntryNotFound = errors.New("collections: entry not found")
	// ErrEntryExists is returned when creating an entry that already exists.
	ErrEntryExists = errors.New("collections: entry already exists")
	// ErrInvalidMove is returned when a folder would move into itself.
	ErrInvalidMove = errors.New("collections: a folder cannot move into itself")
	// ErrReservedName is returned for a folder named like the directory that
	// holds the environments of a collection.
	ErrReservedName = errors.New("collections: this name is reserved for collection environments")
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
	// Color of a collection, from its collection.yaml.
	Color string
}

// InheritedAuthentication is the authentication a request set to inherit
// receives, with the collection or folder that defines it. Authentication is
// always set: it is None when no ancestor defines one.
type InheritedAuthentication struct {
	Authentication *domain.Authentication
	SourcePath     string
	SourceName     string
}

// Repository reads the collection tree from a workspace.
type Repository struct {
	workspaceRoot string
}

// NewRepository returns a repository rooted at the workspace directory.
func NewRepository(workspaceRoot string) *Repository {
	return &Repository{workspaceRoot: workspaceRoot}
}

// environmentsDirectoryName holds the environments of a collection. It is not
// part of the request tree.
const environmentsDirectoryName = "environments"

// isCollectionPath reports whether a cleaned path is a collection directory,
// such as collections/billing.
func isCollectionPath(cleaned string) bool {
	segments := strings.Split(cleaned, "/")
	return len(segments) == 2 && isRoot(segments[0])
}

// isFolderPath reports whether a cleaned path is a folder of a collection, such
// as collections/billing/users, or a folder nested in one, such as
// collections/billing/users/invoices.
func isFolderPath(cleaned string) bool {
	segments := strings.Split(path.Clean(cleaned), "/")
	if len(segments) < 3 || !isRoot(segments[0]) ||
		strings.HasSuffix(segments[len(segments)-1], fileExtension) {
		return false
	}
	for _, segment := range segments[2:] {
		if !isValidName(segment) {
			return false
		}
	}
	return true
}

// isValidName reports whether a path segment is usable as a name.
func isValidName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `\:`)
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

		if entry.IsDir() && entry.Name() == environmentsDirectoryName && isCollectionPath(path.Clean(relativePath)) {
			continue
		}
		if entry.IsDir() {
			kind := KindDirectory
			if relativePath == "" || isRoot(path.Clean(relativePath)) {
				kind = KindCollection
			}

			name := entry.Name()
			color := ""
			if kind == KindCollection {
				collectionName, collectionColor, err := readCollectionName(filepath.Join(directory, entry.Name()), name)
				if err != nil {
					return nil, fmt.Errorf("read collection %s: %w", entryPath, err)
				}
				name = collectionName
				color = collectionColor
			} else {
				name = readFolderName(filepath.Join(directory, entry.Name()), name)
			}

			hasChildren, err := hasChildren(filepath.Join(directory, entry.Name()), kind == KindCollection)
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", entryPath, err)
			}

			result = append(result, Entry{
				Path:        entryPath,
				Name:        name,
				Kind:        kind,
				HasChildren: hasChildren,
				Color:       color,
			})
			continue
		}

		if !strings.HasSuffix(entry.Name(), fileExtension) || isMetadataFile(entry.Name()) {
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
			if entry.Name() == environmentsDirectoryName {
				relativeDirectory, err := filepath.Rel(repository.workspaceRoot, filepath.Dir(filePath))
				if err == nil && isCollectionPath(filepath.ToSlash(relativeDirectory)) {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), fileExtension) || isMetadataFile(entry.Name()) {
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

// ResolveInheritedHeaders returns the default headers a request receives from
// its collection and its folders, weakest first: the collection, then each
// folder down to the one holding the request. A request header with the same
// name overrides them, because headers are applied in order.
func (repository *Repository) ResolveInheritedHeaders(requestPath string) ([]domain.Header, error) {
	if !isRequestPath(requestPath) {
		return nil, ErrInvalidPath
	}

	var layers [][]domain.Header
	directory := path.Dir(path.Clean(requestPath))
	for directory != "." && directory != "/" {
		if !isRoot(directory) {
			metadata, err := repository.LoadCollection(directory)
			if err != nil && !errors.Is(err, ErrEntryNotFound) {
				return nil, err
			}
			if err == nil && len(metadata.Headers) > 0 {
				layers = append(layers, metadata.Headers)
			}
		}

		parent := path.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}

	headers := make([]domain.Header, 0)
	for index := len(layers) - 1; index >= 0; index-- {
		headers = append(headers, layers[index]...)
	}
	return headers, nil
}

// LoadCollection reads and validates the metadata of a collection or folder.
// A collection directory without metadata is valid and returns ErrEntryNotFound
// so callers can fall back to the directory name. A folder without folder.yaml
// falls back to its directory name until its settings are saved.
func (repository *Repository) LoadCollection(relativePath string) (domain.Collection, error) {
	if strings.HasSuffix(relativePath, fileExtension) {
		return domain.Collection{}, ErrInvalidPath
	}

	cleaned := path.Clean(relativePath)
	directory, err := repository.resolve(cleaned)
	if err != nil {
		return domain.Collection{}, err
	}

	if isFolderPath(cleaned) {
		metadata, err := repository.loadFolder(directory)
		if err != nil {
			return domain.Collection{}, err
		}
		return metadata, nil
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

// ResolveInheritedAuthentication returns the authentication of the closest
// ancestor of a request that defines one: its folder, then the folders above
// it, then its collection. A request with no such ancestor receives none.
func (repository *Repository) ResolveInheritedAuthentication(requestPath string) (InheritedAuthentication, error) {
	if !isRequestPath(requestPath) {
		return InheritedAuthentication{}, ErrInvalidPath
	}
	return repository.inheritedAuthenticationFrom(path.Dir(path.Clean(requestPath)))
}

// ResolveInheritedAuthenticationForScope returns the authentication a folder
// inherits from the folders and collection above it. A collection has no
// parent scope and receives none.
func (repository *Repository) ResolveInheritedAuthenticationForScope(scopePath string) (InheritedAuthentication, error) {
	cleaned := path.Clean(scopePath)
	if !isFolderPath(cleaned) {
		return InheritedAuthentication{}, ErrInvalidPath
	}
	return repository.inheritedAuthenticationFrom(path.Dir(cleaned))
}

// inheritedAuthenticationFrom walks the directories from directory to the
// collections root and returns the first authentication that replaces the
// inherited one.
func (repository *Repository) inheritedAuthenticationFrom(directory string) (InheritedAuthentication, error) {
	none := InheritedAuthentication{Authentication: &domain.Authentication{Type: domain.AuthenticationNone}}
	for directory != "." && directory != "/" {
		if !isRoot(directory) {
			metadata, err := repository.LoadCollection(directory)
			if err != nil && !errors.Is(err, ErrEntryNotFound) {
				return InheritedAuthentication{}, err
			}
			if err == nil && metadata.Authentication != nil &&
				metadata.Authentication.Type != domain.AuthenticationInherit {
				copied := *metadata.Authentication
				return InheritedAuthentication{
					Authentication: &copied,
					SourcePath:     directory,
					SourceName:     metadata.Name,
				}, nil
			}
		}

		parent := path.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}

	return none, nil
}

// loadFolder reads the metadata of a folder: its display name, its default
// variables and its authentication. A folder created outside Awake, such as by
// an import, has no folder.yaml yet: it falls back to its directory name until
// its settings are saved.
func (repository *Repository) loadFolder(directory string) (domain.Collection, error) {
	content, err := os.ReadFile(filepath.Join(directory, folderFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			info, statErr := os.Stat(directory)
			if statErr != nil || !info.IsDir() {
				return domain.Collection{}, ErrEntryNotFound
			}
			return domain.Collection{Name: filepath.Base(directory)}, nil
		}
		return domain.Collection{}, fmt.Errorf("read folder metadata %s: %w", directory, err)
	}

	var metadata folderMetadata
	if err := yaml.Unmarshal(content, &metadata); err != nil {
		return domain.Collection{}, fmt.Errorf("%w: %v", domain.ErrInvalidCollection, err)
	}
	if strings.TrimSpace(metadata.Name) == "" {
		return domain.Collection{}, domain.ErrInvalidCollection
	}

	return domain.Collection{
		Name:           metadata.Name,
		Variables:      metadata.Variables,
		Headers:        metadata.Headers,
		Authentication: metadata.Authentication,
		Documentation:  metadata.Documentation,
	}, nil
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
// UpdateCollection replaces the metadata of an existing collection: its
// name, documentation, variables and authentication.
func (repository *Repository) UpdateCollection(relativePath string, collection domain.Collection) error {
	cleaned := path.Clean(relativePath)
	if isFolderPath(cleaned) {
		return repository.updateFolder(cleaned, collection)
	}
	if !isCollectionPath(cleaned) {
		return ErrInvalidPath
	}
	if err := collection.Validate(); err != nil {
		return err
	}

	directory, err := repository.resolve(cleaned)
	if err != nil {
		return err
	}
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return ErrEntryNotFound
	}

	content, err := yamlcodec.EncodeValue(collection)
	if err != nil {
		return fmt.Errorf("encode collection %s: %w", relativePath, err)
	}
	return files.WriteAtomically(filepath.Join(directory, collectionFileName), content)
}

// updateFolder replaces the metadata of an existing folder: its display name,
// its default variables and its authentication, which refine or replace those
// of the collection it belongs to.
func (repository *Repository) updateFolder(cleaned string, collection domain.Collection) error {
	directory, err := repository.resolve(cleaned)
	if err != nil {
		return err
	}
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return ErrEntryNotFound
	}
	if strings.TrimSpace(collection.Name) == "" {
		return ErrInvalidPath
	}

	metadata := folderMetadata{
		Name:           collection.Name,
		Variables:      collection.Variables,
		Headers:        collection.Headers,
		Authentication: collection.Authentication,
		Documentation:  collection.Documentation,
	}
	content, err := yamlcodec.EncodeValue(metadata)
	if err != nil {
		return fmt.Errorf("encode folder %s: %w", cleaned, err)
	}
	return files.WriteAtomically(filepath.Join(directory, folderFileName), content)
}

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

// Move moves a request or a folder into another collection or folder, keeping
// its file name, and returns its new path. Collections themselves stay at the
// root. Moving between collections/ and local/ shares or privatizes an entry.
func (repository *Repository) Move(relativePath string, targetDirectory string) (string, error) {
	source := path.Clean(relativePath)
	target := path.Clean(targetDirectory)
	if source == "." || isRoot(source) || isCollectionPath(source) || isRoot(target) || target == "." {
		return "", ErrInvalidPath
	}

	sourcePath, err := repository.resolve(source)
	if err != nil {
		return "", err
	}
	targetPath, err := repository.resolve(target)
	if err != nil {
		return "", err
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrEntryNotFound
		}
		return "", fmt.Errorf("inspect %s: %w", relativePath, err)
	}
	if !sourceInfo.IsDir() && !isRequestPath(source) {
		return "", ErrInvalidPath
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrEntryNotFound
		}
		return "", fmt.Errorf("inspect %s: %w", targetDirectory, err)
	}
	if !targetInfo.IsDir() {
		return "", ErrInvalidPath
	}

	if path.Dir(source) == target {
		return source, nil
	}
	if sourceInfo.IsDir() && (target == source || strings.HasPrefix(target, source+"/")) {
		return "", ErrInvalidMove
	}

	destination := path.Join(target, path.Base(source))
	if sourceInfo.IsDir() && path.Base(destination) == environmentsDirectoryName && isCollectionPath(target) {
		return "", ErrReservedName
	}
	destinationPath, err := repository.resolve(destination)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(destinationPath); err == nil {
		return "", ErrEntryExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect %s: %w", destination, err)
	}

	if err := os.Rename(sourcePath, destinationPath); err != nil {
		return "", fmt.Errorf("move %s: %w", relativePath, err)
	}
	if !sourceInfo.IsDir() {
		err := os.Rename(overridePath(sourcePath), overridePath(destinationPath))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("move local overrides of %s: %w", relativePath, err)
		}
	}
	return destination, nil
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
		if err := os.Remove(overridePath(target)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete local overrides of %s: %w", relativePath, err)
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
func (repository *Repository) CreateFolder(relativePath string, name string) error {
	cleaned := path.Clean(relativePath)
	if strings.Count(cleaned, "/") < 2 || isRequestPath(cleaned) {
		return ErrInvalidPath
	}
	if path.Base(cleaned) == environmentsDirectoryName && isCollectionPath(path.Dir(cleaned)) {
		return ErrReservedName
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

	if strings.TrimSpace(name) == "" {
		name = path.Base(cleaned)
	}
	content, err := yamlcodec.EncodeValue(folderMetadata{Name: strings.TrimSpace(name)})
	if err != nil {
		return fmt.Errorf("encode folder %s: %w", relativePath, err)
	}
	return files.WriteAtomically(filepath.Join(directory, folderFileName), content)
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

// folderMetadata is the content of folder.yaml.
type folderMetadata struct {
	Name           string                 `yaml:"name"`
	Variables      map[string]string      `yaml:"variables,omitempty"`
	Headers        []domain.Header        `yaml:"headers,omitempty"`
	Authentication *domain.Authentication `yaml:"authentication,omitempty"`
	Documentation  string                 `yaml:"documentation,omitempty"`
}

// isMetadataFile reports whether a file describes its directory rather than
// holding a request.
// isMetadataFile reports a file that sits among requests without being one:
// collection and folder metadata, and the local overrides of a request.
func isMetadataFile(name string) bool {
	return name == collectionFileName || name == folderFileName || strings.HasSuffix(name, localOverrideSuffix)
}

// overridePath returns the path of the local overrides of a request.
func overridePath(requestPath string) string {
	return strings.TrimSuffix(requestPath, fileExtension) + localOverrideSuffix
}

// readFolderName returns the display name stored in folder.yaml, or the
// directory name for folders created outside the application.
func readFolderName(directory string, fallback string) string {
	content, err := os.ReadFile(filepath.Join(directory, folderFileName))
	if err != nil {
		return fallback
	}
	var metadata folderMetadata
	if err := yaml.Unmarshal(content, &metadata); err != nil || strings.TrimSpace(metadata.Name) == "" {
		return fallback
	}
	return metadata.Name
}

// readCollectionName returns the display name and the color of a collection.
func readCollectionName(directory string, fallback string) (string, string, error) {
	content, err := os.ReadFile(filepath.Join(directory, collectionFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fallback, "", nil
		}
		return "", "", err
	}

	var collection domain.Collection
	if err := yaml.Unmarshal(content, &collection); err != nil {
		return "", "", fmt.Errorf("%w: %v", domain.ErrInvalidCollection, err)
	}
	if err := collection.Validate(); err != nil {
		return "", "", err
	}

	return collection.Name, collection.Color, nil
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

func hasChildren(directory string, isCollection bool) (bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() && isCollection && entry.Name() == environmentsDirectoryName {
			continue
		}
		if entry.IsDir() {
			return true, nil
		}
		if strings.HasSuffix(entry.Name(), fileExtension) && !isMetadataFile(entry.Name()) {
			return true, nil
		}
	}

	return false, nil
}

func isRequestPath(relativePath string) bool {
	return strings.HasSuffix(relativePath, fileExtension) &&
		!isMetadataFile(path.Base(relativePath))
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

// LoadRequestOverride reads the local overrides of a request. A request
// without overrides returns an empty override.
func (repository *Repository) LoadRequestOverride(relativePath string) (domain.RequestOverride, error) {
	if !isRequestPath(relativePath) {
		return domain.RequestOverride{}, ErrInvalidPath
	}
	filePath, err := repository.resolve(relativePath)
	if err != nil {
		return domain.RequestOverride{}, err
	}

	content, err := os.ReadFile(overridePath(filePath))
	if errors.Is(err, os.ErrNotExist) {
		return domain.RequestOverride{}, nil
	}
	if err != nil {
		return domain.RequestOverride{}, fmt.Errorf("read local overrides of %s: %w", relativePath, err)
	}

	var override domain.RequestOverride
	if err := yaml.Unmarshal(content, &override); err != nil {
		return domain.RequestOverride{}, fmt.Errorf("%w: local overrides of %s: %v", domain.ErrInvalidRequest, relativePath, err)
	}
	if override.Body != nil {
		override.Body.NormalizeForm()
	}
	return override, nil
}

// SaveRequestOverride writes the local overrides of a request, or removes the
// file when the override replaces nothing.
func (repository *Repository) SaveRequestOverride(relativePath string, override domain.RequestOverride) error {
	if !isRequestPath(relativePath) {
		return ErrInvalidPath
	}
	filePath, err := repository.resolve(relativePath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrEntryNotFound
		}
		return fmt.Errorf("inspect %s: %w", relativePath, err)
	}

	if override.IsEmpty() {
		if err := os.Remove(overridePath(filePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete local overrides of %s: %w", relativePath, err)
		}
		return nil
	}

	content, err := yamlcodec.EncodeValue(override)
	if err != nil {
		return fmt.Errorf("encode local overrides of %s: %w", relativePath, err)
	}
	return files.WriteAtomically(overridePath(filePath), content)
}
