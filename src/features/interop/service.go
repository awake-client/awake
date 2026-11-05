package interop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/clementherve/awake/src/domain"
	"github.com/clementherve/awake/src/shared/files"
)

// Writer persists imported collections and requests. The application layer
// implements it on top of the collections feature.
type Writer interface {
	CreateCollection(workspaceRoot string, relativePath string, collection domain.Collection) error
	CreateRequest(workspaceRoot string, relativePath string, request domain.Request) error
}

// CollectionSnapshot is a collection read from a workspace for export.
type CollectionSnapshot struct {
	Name           string
	Documentation  string
	Variables      map[string]string
	Authentication *domain.Authentication
	Requests       []RequestSnapshot
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

		requests, warnings, err := service.writeRequests(workspaceRoot, collectionPath, collection.Requests, collection.Folders)
		if err != nil {
			return result, err
		}

		result.Requests += requests
		result.Warnings = append(result.Warnings, warnings...)
	}

	if result.Requests == 0 {
		return result, ErrEmptyCollection
	}

	return result, nil
}

// Export writes the workspace collections to destinationPath. OpenAPI and
// Postman produce one file; Bruno produces a directory.
func (service *Service) Export(workspaceRoot string, destinationPath string, format Format) (ExportResult, error) {
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
		content, err := WriteOpenAPI(collections)
		if err != nil {
			return ExportResult{}, err
		}
		return writeExportFile(destinationPath, ".yaml", content)

	case FormatPostman:
		content, err := WritePostman(collections)
		if err != nil {
			return ExportResult{}, err
		}
		return writeExportFile(destinationPath, ".json", content)

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
