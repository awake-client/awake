package interop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/awake-client/awake/src/domain"
)

// pluginFormatPrefix starts the name of a format a plugin provides, written
// plugin:<plugin id>/<format>.
const pluginFormatPrefix = "plugin:"

// defaultPluginExtension names an exported document whose format declares no
// extension.
const defaultPluginExtension = ".txt"

// ErrPluginFormatsUnavailable is returned for a plugin format when no plugin
// host is connected.
var ErrPluginFormatsUnavailable = errors.New("interop: plugin formats are not available in this build")

// PluginFormat is a document format an enabled plugin imports or exports.
type PluginFormat struct {
	Format     Format   `json:"format"`
	Label      string   `json:"label"`
	Extensions []string `json:"extensions"`
}

// PluginFormatList lists the formats the enabled plugins of a workspace add.
type PluginFormatList struct {
	Imports []PluginFormat `json:"imports"`
	Exports []PluginFormat `json:"exports"`
}

// PluginFormats converts documents with the formats plugins provide. Plugins
// exchange collections as JSON shaped like the files of a workspace. The
// application layer supplies the implementation so this feature does not
// depend on the plugins feature.
type PluginFormats interface {
	Formats(workspaceRoot string) (imports []PluginFormat, exports []PluginFormat)
	Import(workspaceRoot string, format Format, fileName string, content []byte) ([]byte, error)
	Export(workspaceRoot string, format Format, collections []byte) (string, error)
}

// SetPluginFormats lets imports and exports use the formats of plugins. It is
// a package function rather than a service method, so the host never crosses
// the Wails bridge.
func SetPluginFormats(service *Service, formats PluginFormats) {
	service.plugins = formats
}

// PluginFormats lists the import and export formats the enabled plugins of
// the workspace add, for the import and export panel.
func (service *Service) PluginFormats(workspaceRoot string) PluginFormatList {
	if service.plugins == nil {
		return PluginFormatList{Imports: []PluginFormat{}, Exports: []PluginFormat{}}
	}
	imports, exports := service.plugins.Formats(workspaceRoot)
	return PluginFormatList{Imports: imports, Exports: exports}
}

// IsPluginFormat reports whether format names a format a plugin provides.
func IsPluginFormat(format Format) bool {
	return strings.HasPrefix(string(format), pluginFormatPrefix)
}

// pluginCollection is the shape of a collection exchanged with a plugin. Its
// requests use the same fields as the request files of a workspace.
type pluginCollection struct {
	Name           string                 `yaml:"name"`
	Documentation  string                 `yaml:"documentation,omitempty"`
	Variables      map[string]string      `yaml:"variables,omitempty"`
	Authentication *domain.Authentication `yaml:"authentication,omitempty"`
	Requests       []pluginRequest        `yaml:"requests,omitempty"`
	Folders        []pluginFolder         `yaml:"folders,omitempty"`
	Environments   []pluginEnvironment    `yaml:"environments,omitempty"`
}

type pluginFolder struct {
	Name          string          `yaml:"name"`
	Documentation string          `yaml:"documentation,omitempty"`
	Requests      []pluginRequest `yaml:"requests,omitempty"`
	Folders       []pluginFolder  `yaml:"folders,omitempty"`
}

type pluginRequest struct {
	Name    string         `yaml:"name"`
	Request domain.Request `yaml:"request"`
}

type pluginEnvironment struct {
	Name            string            `yaml:"name"`
	Variables       map[string]string `yaml:"variables,omitempty"`
	SecretVariables []string          `yaml:"secretVariables,omitempty"`
}

func (service *Service) importWithPlugin(workspaceRoot string, sourcePath string, format Format) ([]Collection, error) {
	if service.plugins == nil {
		return nil, ErrPluginFormatsUnavailable
	}
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sourcePath, err)
	}
	encoded, err := service.plugins.Import(workspaceRoot, format, filepath.Base(sourcePath), content)
	if err != nil {
		return nil, err
	}
	return decodePluginCollections(encoded)
}

func (service *Service) exportWithPlugin(workspaceRoot string, destinationPath string, format Format, collections []Collection) (ExportResult, error) {
	if service.plugins == nil {
		return ExportResult{}, ErrPluginFormatsUnavailable
	}
	_, exports := service.plugins.Formats(workspaceRoot)
	position := slices.IndexFunc(exports, func(candidate PluginFormat) bool { return candidate.Format == format })
	if position < 0 {
		return ExportResult{}, fmt.Errorf("%w: %s", ErrUnsupportedFormat, format)
	}
	extension := defaultPluginExtension
	if len(exports[position].Extensions) > 0 {
		extension = exports[position].Extensions[0]
	}

	encoded, err := encodePluginCollections(collections)
	if err != nil {
		return ExportResult{}, err
	}
	content, err := service.plugins.Export(workspaceRoot, format, encoded)
	if err != nil {
		return ExportResult{}, err
	}
	return writeExportFile(destinationPath, extension, []byte(content))
}

// decodePluginCollections reads the collections a plugin returned. Unknown
// fields are refused, so a misspelt key is reported to the plugin author
// instead of silently dropping a value. A request Awake cannot store is
// skipped with a warning, as the other readers do.
func decodePluginCollections(encoded []byte) ([]Collection, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(encoded))
	decoder.KnownFields(true)
	var decoded []pluginCollection
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("interop: read the collections the plugin returned: %w", err)
	}

	collections := make([]Collection, 0, len(decoded))
	for _, collection := range decoded {
		warnings := []string{}
		requests := pluginRequests(collection.Requests, collection.Name, &warnings)
		folders := pluginFolders(collection.Folders, collection.Name, &warnings)
		environments := make([]Environment, 0, len(collection.Environments))
		for _, environment := range collection.Environments {
			environments = append(environments, Environment{
				Name:            environment.Name,
				Variables:       environment.Variables,
				SecretVariables: environment.SecretVariables,
			})
		}
		collections = append(collections, Collection{
			Name:           collection.Name,
			Documentation:  collection.Documentation,
			Variables:      collection.Variables,
			Authentication: collection.Authentication,
			Requests:       requests,
			Folders:        folders,
			Environments:   environments,
			Warnings:       warnings,
		})
	}
	return collections, nil
}

func pluginRequests(entries []pluginRequest, location string, warnings *[]string) []Request {
	requests := make([]Request, 0, len(entries))
	for _, entry := range entries {
		request := entry.Request
		if request.Name == "" {
			request.Name = entry.Name
		}
		request.ApplyDefaults()
		if err := request.Validate(); err != nil {
			*warnings = append(*warnings, fmt.Sprintf("%s/%s: skipped, %v", location, entry.Name, err))
			continue
		}
		requests = append(requests, Request{Name: entry.Name, Request: request})
	}
	return requests
}

func pluginFolders(entries []pluginFolder, location string, warnings *[]string) []Folder {
	folders := make([]Folder, 0, len(entries))
	for _, entry := range entries {
		path := location + "/" + entry.Name
		folders = append(folders, Folder{
			Name:          entry.Name,
			Documentation: entry.Documentation,
			Requests:      pluginRequests(entry.Requests, path, warnings),
			Folders:       pluginFolders(entry.Folders, path, warnings),
		})
	}
	return folders
}

// encodePluginCollections writes collections as the JSON plugins read, going
// through YAML so the request fields keep the names of the workspace files.
func encodePluginCollections(collections []Collection) ([]byte, error) {
	shaped := make([]pluginCollection, 0, len(collections))
	for _, collection := range collections {
		environments := make([]pluginEnvironment, 0, len(collection.Environments))
		for _, environment := range collection.Environments {
			environments = append(environments, pluginEnvironment{
				Name:            environment.Name,
				Variables:       environment.Variables,
				SecretVariables: environment.SecretVariables,
			})
		}
		shaped = append(shaped, pluginCollection{
			Name:           collection.Name,
			Documentation:  collection.Documentation,
			Variables:      collection.Variables,
			Authentication: collection.Authentication,
			Requests:       shapedRequests(collection.Requests),
			Folders:        shapedFolders(collection.Folders),
			Environments:   environments,
		})
	}

	asYAML, err := yaml.Marshal(shaped)
	if err != nil {
		return nil, fmt.Errorf("interop: encode collections for the plugin: %w", err)
	}
	var generic any
	if err := yaml.Unmarshal(asYAML, &generic); err != nil {
		return nil, fmt.Errorf("interop: encode collections for the plugin: %w", err)
	}
	if generic == nil {
		generic = []any{}
	}
	return json.Marshal(generic)
}

func shapedRequests(requests []Request) []pluginRequest {
	shaped := make([]pluginRequest, 0, len(requests))
	for _, request := range requests {
		shaped = append(shaped, pluginRequest{Name: request.Name, Request: request.Request})
	}
	return shaped
}

func shapedFolders(folders []Folder) []pluginFolder {
	shaped := make([]pluginFolder, 0, len(folders))
	for _, folder := range folders {
		shaped = append(shaped, pluginFolder{
			Name:          folder.Name,
			Documentation: folder.Documentation,
			Requests:      shapedRequests(folder.Requests),
			Folders:       shapedFolders(folder.Folders),
		})
	}
	return shaped
}
