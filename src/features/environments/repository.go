package environments

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/clementherve/awake/src/shared/files"
	"github.com/clementherve/awake/src/shared/yamlcodec"
)

// Repository reads the environment files of one scope: the workspace for
// global environments, or a collection for environments that refine them.
type Repository struct {
	workspaceRoot string
	directory     string
}

// NewRepository returns the repository of the global environments.
func NewRepository(workspaceRoot string) *Repository {
	return &Repository{
		workspaceRoot: workspaceRoot,
		directory:     filepath.Join(workspaceRoot, directoryName),
	}
}

// NewScopedRepository returns the repository of a scope. An empty scope is
// the workspace; otherwise the scope is a collection path such as
// collections/billing or local/scratch, whose environments live in its own
// environments directory.
func NewScopedRepository(workspaceRoot string, scope string) (*Repository, error) {
	if scope == "" {
		return NewRepository(workspaceRoot), nil
	}
	if err := ValidateScope(scope); err != nil {
		return nil, err
	}
	return &Repository{
		workspaceRoot: workspaceRoot,
		directory:     filepath.Join(workspaceRoot, filepath.FromSlash(scope), directoryName),
	}, nil
}

// ValidateScope accepts a collection path: a root, shared or private, and a
// collection directory name.
func ValidateScope(scope string) error {
	segments := strings.Split(scope, "/")
	if len(segments) != 2 || (segments[0] != sharedCollectionsRoot && segments[0] != privateCollectionsRoot) {
		return ErrInvalidScope
	}
	name := segments[1]
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `\:`) {
		return ErrInvalidScope
	}
	return nil
}

// ScopeOfRequest returns the collection a request belongs to, as a scope.
func ScopeOfRequest(requestPath string) string {
	segments := strings.Split(requestPath, "/")
	if len(segments) < 3 {
		return ""
	}
	scope := segments[0] + "/" + segments[1]
	if ValidateScope(scope) != nil {
		return ""
	}
	return scope
}

// DirectoryPath returns the environments directory of the scope.
func (repository *Repository) DirectoryPath() string {
	return repository.directory
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
			Color:             environment.Color,
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

// Save writes the shared environment file with canonical formatting. Existing
// top-level comments are preserved when the file can be parsed.
func (repository *Repository) Save(fileName string, environment Environment) error {
	if err := validateFileName(fileName); err != nil {
		return err
	}
	if err := environment.validate(); err != nil {
		return err
	}

	path := repository.SharedPath(fileName)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read environment %s: %w", fileName, err)
	}

	var content []byte
	if len(existing) > 0 {
		content, err = encodeEnvironment(existing, environment)
	} else {
		content, err = yamlcodec.EncodeValue(environment)
	}
	if err != nil {
		return fmt.Errorf("encode environment %s: %w", fileName, err)
	}

	if err := os.MkdirAll(repository.DirectoryPath(), 0o755); err != nil {
		return fmt.Errorf("create environments directory: %w", err)
	}

	return files.WriteAtomically(path, content)
}

func encodeEnvironment(existing []byte, environment Environment) ([]byte, error) {
	document, err := yamlcodec.Parse(existing)
	if err != nil || len(document.Root().Content) == 0 {
		return yamlcodec.EncodeValue(environment)
	}

	mapping := document.Root().Content[0]
	if mapping.Kind != yaml.MappingNode {
		return yamlcodec.EncodeValue(environment)
	}

	setScalar(mapping, "name", environment.Name)
	if environment.Color == "" {
		removeMappingValue(mapping, "color")
	} else {
		setScalar(mapping, "color", environment.Color)
	}
	setStringMap(mapping, "variables", environment.Variables)
	setStringSequence(mapping, "secretVariables", environment.SecretVariables)

	return document.Encode()
}

func setScalar(mapping *yaml.Node, key string, value string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1].Kind = yaml.ScalarNode
			mapping.Content[index+1].Tag = "!!str"
			mapping.Content[index+1].Value = value
			mapping.Content[index+1].Content = nil
			return
		}
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func setStringMap(mapping *yaml.Node, key string, values map[string]string) {
	if len(values) == 0 {
		removeMappingValue(mapping, key)
		return
	}

	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, name := range keys {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: values[name]},
		)
	}

	setMappingValue(mapping, key, node)
}

func setStringSequence(mapping *yaml.Node, key string, values []string) {
	if len(values) == 0 {
		removeMappingValue(mapping, key)
		return
	}

	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: value,
		})
	}

	setMappingValue(mapping, key, node)
}

func setMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content[index+1] = value
			return
		}
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

func removeMappingValue(mapping *yaml.Node, key string) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
}

// Delete removes a shared environment and its local overrides.
func (repository *Repository) Delete(fileName string) error {
	if err := validateFileName(fileName); err != nil {
		return err
	}

	sharedPath := repository.SharedPath(fileName)
	if !fileExists(sharedPath) {
		return ErrEnvironmentNotFound
	}
	if err := os.Remove(sharedPath); err != nil {
		return fmt.Errorf("delete environment %s: %w", fileName, err)
	}
	if err := os.Remove(repository.LocalPath(fileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete local overrides of %s: %w", fileName, err)
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
