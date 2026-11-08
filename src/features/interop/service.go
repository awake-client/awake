package interop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/shared/files"
)

// Writer persists imported collections and requests. The application layer
// implements it on top of the collections feature.
type Writer interface {
	CreateCollection(workspaceRoot string, relativePath string, collection domain.Collection) error
	CreateRequest(workspaceRoot string, relativePath string, request domain.Request) error
	// CreateEnvironment writes an environment of a collection, scope being
	// the collection path. It returns os.ErrExist rather than replace one.
	CreateEnvironment(workspaceRoot string, scope string, fileName string, environment Environment) error
}

// CollectionSnapshot is a collection read from a workspace for export.
type CollectionSnapshot struct {
	Name           string
	Documentation  string
	Variables      map[string]string
	Authentication *domain.Authentication
	Requests       []RequestSnapshot
	Environments   []Environment
}

// ExportOptions tunes how an export is written.
type ExportOptions struct {
	// SplitCollections writes one document per collection instead of one
	// document for the whole workspace. Bruno always writes one directory per
	// collection.
	SplitCollections bool
}

// RequestSnapshot is a request read from a workspace. RelativePath is relative
// to the collection directory.
type RequestSnapshot struct {
	RelativePath string
	Request      domain.Request
}

// Source reads the collections of a workspace for export. The application
// layer implements it on top of the collections feature.
type Source interface {
	LoadCollections(workspaceRoot string) ([]CollectionSnapshot, error)
}

// Service imports and exports workspace collections.
type Service struct {
	writer Writer
	source Source
}

// NewService returns an interop service backed by writer and source.
func NewService(writer Writer, source Source) *Service {
	return &Service{writer: writer, source: source}
}

// Import reads an OpenAPI, Postman or Bruno document and writes it into the
// collections of a workspace. An empty format is detected from the file.
func (service *Service) Import(workspaceRoot string, sourcePath string, format Format) (ImportResult, error) {
	collections, err := readCollections(sourcePath, format)
	if err != nil {
		return ImportResult{}, err
	}
	if len(collections) == 0 {
		return ImportResult{}, ErrEmptyCollection
	}

	result := ImportResult{}
	usedCollections := map[string]int{}

	for _, collection := range collections {
		collectionName := uniqueName(usedCollections, slugify(collection.Name), "collection")
		collectionPath := "collections/" + collectionName

		if err := service.writer.CreateCollection(workspaceRoot, collectionPath, domain.Collection{
			Name:           collection.Name,
			Variables:      collection.Variables,
			Authentication: collection.Authentication,
			Documentation:  collection.Documentation,
		}); err != nil && !errors.Is(err, os.ErrExist) {
			return result, fmt.Errorf("create collection %s: %w", collection.Name, err)
		}
		result.Collections += 1
		result.Warnings = append(result.Warnings, collection.Warnings...)

		requests, warnings, err := service.writeRequests(workspaceRoot, collectionPath, collection.Requests, collection.Folders)
		if err != nil {
			return result, err
		}

		result.Requests += requests
		result.Warnings = append(result.Warnings, warnings...)

		environments, warnings, err := service.writeEnvironments(workspaceRoot, collectionPath, collection.Environments)
		if err != nil {
			return result, err
		}
		result.Environments += environments
		result.Warnings = append(result.Warnings, warnings...)
	}

	if result.Requests == 0 {
		return result, ErrEmptyCollection
	}

	return result, nil
}

// Export writes the workspace collections to destinationPath. OpenAPI and
// Postman produce one file, or one file per collection when options asks for
// it; Bruno produces a directory. Postman and Bruno also write the
// environments of each collection.
func (service *Service) Export(workspaceRoot string, destinationPath string, format Format, options ExportOptions) (ExportResult, error) {
	snapshots, err := service.source.LoadCollections(workspaceRoot)
	if err != nil {
		return ExportResult{}, err
	}
	if len(snapshots) == 0 {
		return ExportResult{}, ErrEmptyCollection
	}

	collections := make([]Collection, 0, len(snapshots))
	for _, snapshot := range snapshots {
		collections = append(collections, snapshotCollection(snapshot))
	}

	switch format {
	case FormatOpenAPI:
		if options.SplitCollections {
			return writeExportDocuments(destinationPath, ".yaml", collections, WriteOpenAPICollection)
		}
		content, err := WriteOpenAPI(collections)
		if err != nil {
			return ExportResult{}, err
		}
		return writeExportFile(destinationPath, ".yaml", content)

	case FormatPostman:
		return exportPostman(destinationPath, collections, options)

	case FormatBruno:
		paths, err := WriteBrunoDirectory(destinationPath, collections)
		if err != nil {
			return ExportResult{}, err
		}
		return ExportResult{Paths: paths}, nil

	default:
		return ExportResult{}, ErrUnsupportedFormat
	}
}

func exportPostman(destinationPath string, collections []Collection, options ExportOptions) (ExportResult, error) {
	if options.SplitCollections {
		result, err := writeExportDocuments(destinationPath, ".postman.json", collections, WritePostmanCollection)
		if err != nil {
			return result, err
		}

		paths, err := writePostmanEnvironments(filepath.Join(destinationPath, "environments"), collections)
		if err != nil {
			return ExportResult{}, err
		}
		result.Paths = append(result.Paths, paths...)
		return result, nil
	}

	content, err := WritePostman(collections)
	if err != nil {
		return ExportResult{}, err
	}
	result, err := writeExportFile(destinationPath, ".json", content)
	if err != nil {
		return result, err
	}

	// Environments are separate documents in Postman, so they sit in a
	// folder of their own beside the collection file.
	exportPath := result.Paths[0]
	environmentDirectory := strings.TrimSuffix(exportPath, filepath.Ext(exportPath)) + ".environments"
	paths, err := writePostmanEnvironments(environmentDirectory, collections)
	if err != nil {
		return ExportResult{}, err
	}
	result.Paths = append(result.Paths, paths...)
	return result, nil
}

func readCollections(sourcePath string, format Format) ([]Collection, error) {
	if format == FormatBruno {
		return ReadBrunoDirectory(sourcePath)
	}

	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sourcePath, err)
	}

	if format == "" {
		format = detectDocumentFormat(sourcePath, content)
	}

	switch format {
	case FormatPostman:
		return ReadPostman(content)
	case FormatOpenAPI:
		return ReadOpenAPI(content)
	case FormatBruno:
		request, err := ReadBrunoFile(content, strings.TrimSuffix(filepath.Base(sourcePath), brunoExtension))
		if err != nil {
			return nil, err
		}
		return []Collection{{
			Name:     request.Name,
			Requests: []Request{{Name: request.Name, Request: request}},
		}}, nil
	default:
		return nil, ErrUnsupportedFormat
	}
}

// writeEnvironments writes the environments of an imported collection. A
// variable whose name Awake cannot use is skipped with a warning, and an
// existing environment is kept.
func (service *Service) writeEnvironments(workspaceRoot string, collectionPath string, environments []Environment) (int, []string, error) {
	written := 0
	warnings := make([]string, 0)
	used := map[string]int{}

	for _, environment := range environments {
		fileName := uniqueName(used, slugify(environment.Name), "environment")
		cleaned := Environment{Name: environment.Name, Variables: map[string]string{}}
		for name, value := range environment.Variables {
			if !isVariableName(name) {
				warnings = append(warnings, fmt.Sprintf("environment %s: variable %q skipped, its name is not a valid identifier", environment.Name, name))
				continue
			}
			cleaned.Variables[name] = value
		}
		for _, name := range environment.SecretVariables {
			if !isVariableName(name) {
				warnings = append(warnings, fmt.Sprintf("environment %s: secret %q skipped, its name is not a valid identifier", environment.Name, name))
				continue
			}
			cleaned.SecretVariables = append(cleaned.SecretVariables, name)
		}
		if len(cleaned.SecretVariables) > 0 {
			warnings = append(warnings, fmt.Sprintf("environment %s: secret values are not part of the export, store them in the keychain: %s", environment.Name, strings.Join(cleaned.SecretVariables, ", ")))
		}

		err := service.writer.CreateEnvironment(workspaceRoot, collectionPath, fileName, cleaned)
		if errors.Is(err, os.ErrExist) {
			warnings = append(warnings, fmt.Sprintf("environment %s: %s/environments/%s.yaml already exists and was kept", environment.Name, collectionPath, fileName))
			continue
		}
		if err != nil {
			return written, warnings, fmt.Errorf("create environment %s: %w", environment.Name, err)
		}
		written += 1
	}
	return written, warnings, nil
}

// isVariableName mirrors the variable names environments accept.
func isVariableName(name string) bool {
	for index, character := range name {
		switch {
		case character == '_', character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z':
		case index > 0 && character >= '0' && character <= '9':
		default:
			return false
		}
	}
	return name != ""
}

func (service *Service) writeRequests(
	workspaceRoot string,
	collectionPath string,
	requests []Request,
	folders []Folder,
) (int, []string, error) {
	written := 0
	warnings := make([]string, 0)

	result, folderWarnings, err := service.writeFolder(workspaceRoot, collectionPath, requests, folders)
	if err != nil {
		return written, warnings, err
	}

	return result, append(warnings, folderWarnings...), nil
}

func (service *Service) writeFolder(
	workspaceRoot string,
	directoryPath string,
	requests []Request,
	folders []Folder,
) (int, []string, error) {
	written := 0
	warnings := make([]string, 0)
	usedRequests := map[string]int{}

	for _, request := range requests {
		if strings.TrimSpace(request.Request.URL) == "" {
			warnings = append(warnings, fmt.Sprintf("skipped %q: no URL", request.Name))
			continue
		}
		if strings.TrimSpace(request.Name) == "" {
			warnings = append(warnings, fmt.Sprintf("skipped a request without a name in %s", directoryPath))
			continue
		}

		fileName := uniqueName(usedRequests, slugify(request.Name), "request") + ".yaml"
		requestPath := directoryPath + "/" + fileName

		request.Request.ApplyDefaults()
		if err := service.writer.CreateRequest(workspaceRoot, requestPath, request.Request); err != nil {
			return written, warnings, fmt.Errorf("create request %s: %w", requestPath, err)
		}
		written += 1
	}

	usedFolders := map[string]int{}
	for _, folder := range folders {
		folderName := uniqueName(usedFolders, slugify(folder.Name), "folder")
		folderPath := directoryPath + "/" + folderName

		folderWritten, folderWarnings, err := service.writeFolder(workspaceRoot, folderPath, folder.Requests, folder.Folders)
		if err != nil {
			return written, warnings, err
		}

		written += folderWritten
		warnings = append(warnings, folderWarnings...)
	}

	return written, warnings, nil
}

func snapshotCollection(snapshot CollectionSnapshot) Collection {
	collection := Collection{
		Name:           snapshot.Name,
		Documentation:  snapshot.Documentation,
		Variables:      snapshot.Variables,
		Authentication: snapshot.Authentication,
		Environments:   snapshot.Environments,
		Folders:        []Folder{},
	}

	builder := newFolderTree()
	for _, request := range snapshot.Requests {
		builder.add(request.RelativePath, Request{
			Name:    request.Request.Name,
			Request: request.Request,
		})
	}

	collection.Requests, collection.Folders = builder.result()
	return collection
}

type folderTree struct {
	requests []Request
	folders  map[string]*folderNode
	order    []string
}

type folderNode struct {
	tree *folderTree
}

func newFolderTree() *folderTree {
	return &folderTree{folders: map[string]*folderNode{}}
}

func (tree *folderTree) add(relativePath string, request Request) {
	segments := strings.Split(filepath.ToSlash(relativePath), "/")
	if len(segments) <= 1 {
		tree.requests = append(tree.requests, request)
		return
	}

	current := tree
	for _, segment := range segments[:len(segments)-1] {
		if segment == "" || segment == "." {
			continue
		}

		node, found := current.folders[segment]
		if !found {
			node = &folderNode{tree: newFolderTree()}
			current.folders[segment] = node
			current.order = append(current.order, segment)
		}
		current = node.tree
	}

	current.requests = append(current.requests, request)
}

func (tree *folderTree) result() ([]Request, []Folder) {
	folders := make([]Folder, 0, len(tree.order))
	for _, name := range tree.order {
		node := tree.folders[name]
		requests, children := node.tree.result()
		folders = append(folders, Folder{
			Name:     name,
			Requests: requests,
			Folders:  children,
		})
	}

	return tree.requests, folders
}

func writeExportFile(destinationPath string, extension string, content []byte) (ExportResult, error) {
	if filepath.Ext(destinationPath) == "" {
		destinationPath += extension
	}

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("create export directory: %w", err)
	}

	if err := files.WriteAtomically(destinationPath, content); err != nil {
		return ExportResult{}, err
	}

	return ExportResult{Paths: []string{destinationPath}}, nil
}

// writeExportDocuments writes one document per collection into destination.
func writeExportDocuments(
	destination string,
	extension string,
	collections []Collection,
	write func(Collection) ([]byte, error),
) (ExportResult, error) {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("create export directory: %w", err)
	}

	written := make([]string, 0, len(collections))
	used := map[string]int{}

	for _, collection := range collections {
		content, err := write(collection)
		if err != nil {
			return ExportResult{}, err
		}

		fileName := uniqueName(used, slugify(collection.Name), "collection") + extension
		destinationPath := filepath.Join(destination, fileName)
		if err := files.WriteAtomically(destinationPath, content); err != nil {
			return ExportResult{}, err
		}
		written = append(written, destinationPath)
	}

	return ExportResult{Paths: written}, nil
}

// writePostmanEnvironments writes the environments of every collection in the
// Postman environment format, prefixed by their collection to stay apart.
func writePostmanEnvironments(directory string, collections []Collection) ([]string, error) {
	written := make([]string, 0)
	used := map[string]int{}

	for _, collection := range collections {
		for _, environment := range collection.Environments {
			content, err := WritePostmanEnvironment(environment)
			if err != nil {
				return nil, fmt.Errorf("write postman environment %s: %w", environment.Name, err)
			}

			if err := os.MkdirAll(directory, 0o755); err != nil {
				return nil, fmt.Errorf("create postman environments directory: %w", err)
			}

			fileName := uniqueName(
				used,
				slugify(collection.Name+"-"+environment.Name),
				"environment",
			) + ".postman_environment.json"
			destinationPath := filepath.Join(directory, fileName)
			if err := files.WriteAtomically(destinationPath, content); err != nil {
				return nil, err
			}
			written = append(written, destinationPath)
		}
	}

	return written, nil
}
