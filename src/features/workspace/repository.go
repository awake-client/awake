package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/clementherve/awake/src/shared/yamlcodec"
)

const (
	configurationFileName    = "awake.yaml"
	environmentDirectoryName = "environments"
	collectionDirectoryName  = "collections"
	localDirectoryName       = "local"
	indexDirectoryName       = ".awake"
	ignoreFileName           = ".gitignore"
)

var (
	// ErrWorkspaceNotFound is returned when a directory has no awake.yaml.
	ErrWorkspaceNotFound = errors.New("workspace: configuration file not found")
	// ErrWorkspaceExists is returned when creating a workspace over an
	// existing one.
	ErrWorkspaceExists = errors.New("workspace: configuration file already exists")
	// ErrUnsupportedVersion is returned when the file format is newer than
	// this build supports.
	ErrUnsupportedVersion = errors.New("workspace: unsupported format version")
	// ErrInvalidConfiguration is returned when awake.yaml is malformed.
	ErrInvalidConfiguration = errors.New("workspace: invalid configuration")
)

var ignoreEntries = []string{
	"*.local.yaml",
	"local/",
	".awake/",
}

// Repository reads and writes awake.yaml.
type Repository struct {
	rootPath string
}

// NewRepository returns a repository rooted at rootPath.
func NewRepository(rootPath string) *Repository {
	return &Repository{rootPath: rootPath}
}

// RootPath returns the workspace directory.
func (repository *Repository) RootPath() string {
	return repository.rootPath
}

// ConfigurationPath returns the absolute path of awake.yaml.
func (repository *Repository) ConfigurationPath() string {
	return filepath.Join(repository.rootPath, configurationFileName)
}

// Load reads and validates the workspace configuration.
func (repository *Repository) Load() (Configuration, error) {
	content, err := os.ReadFile(repository.ConfigurationPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Configuration{}, ErrWorkspaceNotFound
		}
		return Configuration{}, fmt.Errorf("read workspace configuration: %w", err)
	}

	var configuration Configuration
	if err := yaml.Unmarshal(content, &configuration); err != nil {
		return Configuration{}, fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
	}

	if err := configuration.validate(); err != nil {
		return Configuration{}, err
	}

	return configuration, nil
}

// Save writes the configuration using the canonical YAML formatting.
func (repository *Repository) Save(configuration Configuration) error {
	if err := configuration.validate(); err != nil {
		return err
	}

	content, err := yamlcodec.EncodeValue(configuration)
	if err != nil {
		return fmt.Errorf("encode workspace configuration: %w", err)
	}

	return writeFileAtomically(repository.ConfigurationPath(), content)
}

// CreateOptions describes a new workspace.
type CreateOptions struct {
	Name string
	Mode Mode
	Git  *GitConfiguration
}

// Create initializes a workspace directory and returns its configuration.
func Create(rootPath string, options CreateOptions) (Configuration, error) {
	if strings.TrimSpace(options.Name) == "" {
		return Configuration{}, ErrInvalidConfiguration
	}
	if options.Mode != ModeTeam && options.Mode != ModePersonal {
		return Configuration{}, ErrInvalidConfiguration
	}

	repository := NewRepository(rootPath)
	if _, err := os.Stat(repository.ConfigurationPath()); err == nil {
		return Configuration{}, ErrWorkspaceExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return Configuration{}, fmt.Errorf("inspect workspace directory: %w", err)
	}

	identifier, err := newIdentifier()
	if err != nil {
		return Configuration{}, err
	}

	configuration := Configuration{
		Version: SupportedVersion,
		ID:      identifier,
		Name:    options.Name,
		Mode:    options.Mode,
		Git:     options.Git,
	}
	if err := configuration.validate(); err != nil {
		return Configuration{}, err
	}

	for _, directory := range []string{
		environmentDirectoryName,
		collectionDirectoryName,
		localDirectoryName,
		indexDirectoryName,
	} {
		path := filepath.Join(rootPath, directory)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return Configuration{}, fmt.Errorf("create workspace directory %s: %w", path, err)
		}
	}

	if err := repository.Save(configuration); err != nil {
		return Configuration{}, err
	}

	if err := ensureIgnoreFile(rootPath); err != nil {
		return Configuration{}, err
	}

	return configuration, nil
}

func newIdentifier() (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate workspace identifier: %w", err)
	}

	return "workspace_" + hex.EncodeToString(randomBytes), nil
}

func ensureIgnoreFile(rootPath string) error {
	path := filepath.Join(rootPath, ignoreFileName)

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read .gitignore: %w", err)
	}

	present := make(map[string]bool)
	for _, line := range strings.Split(string(existing), "\n") {
		present[strings.TrimSpace(line)] = true
	}

	missing := make([]string, 0, len(ignoreEntries))
	for _, entry := range ignoreEntries {
		if !present[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	content := string(existing)
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n"
	}
	content += "# Added by Awake: local overrides and disposable data.\n"
	content += strings.Join(missing, "\n") + "\n"

	return writeFileAtomically(path, []byte(content))
}

func writeFileAtomically(path string, content []byte) error {
	directory := filepath.Dir(path)

	temporaryFile, err := os.CreateTemp(directory, ".awake-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporaryFile.Name()

	defer func() {
		_ = os.Remove(temporaryPath)
	}()

	if _, err := temporaryFile.Write(content); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return fmt.Errorf("set permissions on %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}

	return nil
}
