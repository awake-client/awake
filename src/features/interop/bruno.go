package interop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/clementherve/awake/src/domain"
)

const (
	brunoExtension          = ".bru"
	brunoCollectionFileName = "collection.bru"
	brunoFolderFileName     = "folder.bru"
	// brunoManifestFileName marks every Bruno collection; collection.bru is
	// optional.
	brunoManifestFileName = "bruno.json"
	// brunoEnvironmentDirectory holds the environments of a collection, at
	// its root; its files are variables, not requests.
	brunoEnvironmentDirectory = "environments"
)

var brunoMethodBlocks = map[string]string{
	"get":     domain.MethodGet,
	"post":    domain.MethodPost,
	"put":     domain.MethodPut,
	"patch":   domain.MethodPatch,
	"delete":  domain.MethodDelete,
	"head":    domain.MethodHead,
	"options": domain.MethodOptions,
}

type brunoBlock struct {
	Name  string
	Lines []string
}

type brunoFolderBuilder struct {
	name     string
	requests []Request
	folders  map[string]*brunoFolderBuilder
	order    []string
}

// ReadBrunoDirectory parses a Bruno collection directory, either the original
// .bru layout or the OpenCollection YAML layout. A directory that contains
// collection.bru or opencollection.yml is one collection; otherwise each child
// directory holding one of those files is read as a collection, and a plain
// directory is read as a single unnamed collection.
func ReadBrunoDirectory(root string) ([]Collection, error) {
	collectionRoots, err := brunoCollectionRoots(root)
	if err != nil {
		return nil, err
	}

	collections := make([]Collection, 0, len(collectionRoots))
	for _, collectionRoot := range collectionRoots {
		collection, err := readBrunoCollection(collectionRoot)
		if err != nil {
			return nil, err
		}
		collections = append(collections, collection)
	}

	if len(collections) == 0 {
		return nil, ErrEmptyCollection
	}

	return collections, nil
}

// ReadBrunoFile parses one .bru request file.
func ReadBrunoFile(content []byte, name string) (domain.Request, error) {
	request, err := parseBrunoRequest(string(content), name)
	if err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

func brunoCollectionRoots(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect bruno directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("bruno: %s is not a directory", root)
	}

	if isBrunoCollectionRoot(root) {
		return []string{root}, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read bruno directory: %w", err)
	}

	roots := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if isBrunoCollectionRoot(candidate) {
			roots = append(roots, candidate)
		}
	}

	if len(roots) == 0 {
		return []string{root}, nil
	}

	sort.Strings(roots)
	return roots, nil
}

func isBrunoCollectionRoot(directory string) bool {
	return fileExists(filepath.Join(directory, brunoCollectionFileName)) ||
		fileExists(filepath.Join(directory, brunoManifestFileName)) ||
		isBrunoYAMLCollectionRoot(directory)
}

func isBrunoYAMLCollectionRoot(directory string) bool {
	return fileExists(filepath.Join(directory, brunoYAMLCollectionFileName)) ||
		fileExists(filepath.Join(directory, "opencollection.yaml"))
}

// brunoManifest is the part of bruno.json the import uses.
type brunoManifest struct {
	Name   string   `json:"name"`
	Ignore []string `json:"ignore"`
}

func readBrunoManifest(root string) (brunoManifest, error) {
	content, err := os.ReadFile(filepath.Join(root, brunoManifestFileName))
	if errors.Is(err, os.ErrNotExist) {
		return brunoManifest{}, nil
	}
	if err != nil {
		return brunoManifest{}, fmt.Errorf("read %s: %w", brunoManifestFileName, err)
	}
	var manifest brunoManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return brunoManifest{}, fmt.Errorf("read %s: %w", brunoManifestFileName, err)
	}
	return manifest, nil
}

func readBrunoCollection(root string) (Collection, error) {
	if !fileExists(filepath.Join(root, brunoCollectionFileName)) && isBrunoYAMLCollectionRoot(root) {
		return readBrunoYAMLCollection(root)
	}

	manifest, err := readBrunoManifest(root)
	if err != nil {
		return Collection{}, err
	}
	ignored := map[string]bool{".git": true}
	for _, name := range manifest.Ignore {
		ignored[name] = true
	}

	collection := Collection{
		Name:      filepath.Base(root),
		Variables: map[string]string{},
	}
	if strings.TrimSpace(manifest.Name) != "" {
		collection.Name = manifest.Name
	}

	configurationPath := filepath.Join(root, brunoCollectionFileName)
	if content, err := os.ReadFile(configurationPath); err == nil {
		metadata, err := parseBrunoCollectionFile(string(content), collection.Name)
		if err != nil {
			return Collection{}, fmt.Errorf("read %s: %w", configurationPath, err)
		}
		collection.Name = metadata.Name
		collection.Documentation = metadata.Documentation
		collection.Variables = metadata.Variables
		collection.Authentication = metadata.Authentication
	} else if !errors.Is(err, os.ErrNotExist) {
		return Collection{}, fmt.Errorf("read %s: %w", configurationPath, err)
	}

	builder := &brunoFolderBuilder{
		name:    collection.Name,
		folders: map[string]*brunoFolderBuilder{},
	}

	skipped := map[string][]string{}
	err = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() && filePath != root && ignored[entry.Name()] {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			if filepath.Dir(filePath) == filepath.Clean(root) && entry.Name() == brunoEnvironmentDirectory {
				environments, err := readBrunoEnvironments(filePath)
				if err != nil {
					return err
				}
				collection.Environments = environments
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), brunoExtension) {
			return nil
		}
		if entry.Name() == brunoCollectionFileName || entry.Name() == brunoFolderFileName {
			return nil
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}

		requestName := strings.TrimSuffix(entry.Name(), brunoExtension)
		request, err := parseBrunoRequest(string(content), requestName)
		if err != nil {
			// One request Awake cannot read must not stop the others.
			var unsupported unsupportedBrunoRequest
			if errors.As(err, &unsupported) {
				skipped[unsupported.kind] = append(skipped[unsupported.kind], filepath.ToSlash(relativePath))
				return nil
			}
			collection.Warnings = append(collection.Warnings, fmt.Sprintf("%s skipped: %v", filepath.ToSlash(relativePath), err))
			return nil
		}

		requestName = brunoRequestName(string(content), requestName)
		builder.add(filepath.Dir(relativePath), Request{Name: requestName, Request: request})

		return nil
	})
	if err != nil {
		return Collection{}, fmt.Errorf("read bruno collection: %w", err)
	}

	collection.Requests, collection.Folders = builder.result()
	collection.Warnings = append(collection.Warnings, skipWarnings(skipped)...)

	if len(collection.Requests) == 0 && len(collection.Folders) == 0 {
		return Collection{}, ErrEmptyCollection
	}

	return collection, nil
}

// readBrunoEnvironments parses the .bru files of a collection's environments
// directory, sorted by name.
func readBrunoEnvironments(directory string) ([]Environment, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read bruno environments: %w", err)
	}

	environments := make([]Environment, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), brunoExtension) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read bruno environment %s: %w", entry.Name(), err)
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		environments = append(environments, parseBrunoEnvironment(string(content), name))
	}
	return environments, nil
}

// parseBrunoEnvironment reads a vars block and a vars:secret list. Disabled
// variables are skipped, as Bruno does not apply them.
func parseBrunoEnvironment(content string, name string) Environment {
	environment := Environment{Name: name, Variables: map[string]string{}}

	for _, block := range parseBrunoBlocks(content) {
		if block.Name != "vars" {
			continue
		}
		for _, entry := range brunoKeyValues(block.Lines) {
			if !entry.disabled {
				environment.Variables[entry.key] = entry.value
			}
		}
	}

	for _, secretName := range brunoList(content, "vars:secret") {
		delete(environment.Variables, secretName)
		environment.SecretVariables = append(environment.SecretVariables, secretName)
	}
	return environment
}

// brunoList reads a bracketed list such as vars:secret [ token, password ].
func brunoList(content string, name string) []string {
	lines := strings.Split(content, "\n")
	values := make([]string, 0)
	for index := 0; index < len(lines); index += 1 {
		if strings.TrimSpace(lines[index]) != name+" [" {
			continue
		}
		for index += 1; index < len(lines); index += 1 {
			item := strings.TrimSpace(lines[index])
			if item == "]" {
				break
			}
			item = strings.TrimSpace(strings.TrimSuffix(item, ","))
			if item != "" && !strings.HasPrefix(item, "~") {
				values = append(values, unquote(item))
			}
		}
	}
	return values
}

// skipWarnings sums up the requests skipped by kind, naming a few of them.
func skipWarnings(skipped map[string][]string) []string {
	kinds := make([]string, 0, len(skipped))
	for kind := range skipped {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)

	warnings := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		paths := skipped[kind]
		sort.Strings(paths)
		listed := strings.Join(paths[:min(len(paths), maximumListedSkips)], ", ")
		if len(paths) > maximumListedSkips {
			listed += fmt.Sprintf(" and %d more", len(paths)-maximumListedSkips)
		}
		noun := "request"
		if len(paths) > 1 {
			noun = "requests"
		}
		warnings = append(warnings, fmt.Sprintf("%d %s %s skipped, %v: %s", len(paths), kind, noun, errUnsupportedBrunoRequest, listed))
	}
	return warnings
}

func (builder *brunoFolderBuilder) add(relativeDirectory string, request Request) {
	if relativeDirectory == "." || relativeDirectory == "" {
		builder.requests = append(builder.requests, request)
		return
	}

	segments := strings.Split(filepath.ToSlash(relativeDirectory), "/")
	current := builder
	for _, segment := range segments {
		next, found := current.folders[segment]
		if !found {
			next = &brunoFolderBuilder{
				name:    segment,
				folders: map[string]*brunoFolderBuilder{},
			}
			current.folders[segment] = next
			current.order = append(current.order, segment)
		}
		current = next
	}

	current.requests = append(current.requests, request)
}

func (builder *brunoFolderBuilder) result() ([]Request, []Folder) {
	folders := make([]Folder, 0, len(builder.order))
	for _, name := range builder.order {
		child := builder.folders[name]
		requests, children := child.result()
		folders = append(folders, Folder{
			Name:     child.name,
			Requests: requests,
			Folders:  children,
		})
	}

	return builder.requests, folders
}

// errUnsupportedBrunoRequest marks a request of a kind Awake cannot hold,
// such as gRPC; the import skips it with a warning.
var errUnsupportedBrunoRequest = errors.New("not supported by Awake")

// unsupportedBrunoRequest names the kind of a request Awake cannot hold.
type unsupportedBrunoRequest struct{ kind string }

func (failure unsupportedBrunoRequest) Error() string {
	return failure.kind + " request, " + errUnsupportedBrunoRequest.Error()
}

func (failure unsupportedBrunoRequest) Unwrap() error { return errUnsupportedBrunoRequest }

// maximumListedSkips bounds the files named in one skip warning.
const maximumListedSkips = 3

// brunoUnsupportedBlocks are the request blocks of kinds Awake cannot send.
var brunoUnsupportedBlocks = map[string]string{"grpc": "gRPC"}

func parseBrunoRequest(content string, fallbackName string) (domain.Request, error) {
	blocks := parseBrunoBlocks(content)
	for _, block := range blocks {
		if kind, unsupported := brunoUnsupportedBlocks[block.Name]; unsupported {
			return domain.Request{}, unsupportedBrunoRequest{kind: kind}
		}
	}

	request := domain.Request{
		Name:           fallbackName,
		Method:         domain.MethodGet,
		Authentication: &domain.Authentication{Type: domain.AuthenticationInherit},
		Body:           &domain.Body{Type: domain.BodyNone},
	}

	for _, block := range blocks {
		switch {
		case block.Name == "meta":
			if name := keyValue(block.Lines, "name"); name != "" {
				request.Name = name
			}
		case brunoMethodBlocks[block.Name] != "":
			request.Method = brunoMethodBlocks[block.Name]
			request.URL = keyValue(block.Lines, "url")
		case block.Name == "params:path":
			for _, entry := range brunoKeyValues(block.Lines) {
				request.PathParameters = append(request.PathParameters, domain.PathParameter{Name: entry.key, Value: entry.value})
			}
		case block.Name == "params:query":
			for _, entry := range brunoKeyValues(block.Lines) {
				request.Parameters = append(request.Parameters, domain.Parameter{
					Name:    entry.key,
					Value:   entry.value,
					Enabled: !entry.disabled,
				})
			}
		case block.Name == "headers":
			for _, entry := range brunoKeyValues(block.Lines) {
				request.Headers = append(request.Headers, domain.Header{
					Name:    entry.key,
					Value:   entry.value,
					Enabled: !entry.disabled,
				})
			}
		case block.Name == "auth:bearer":
			request.Authentication = &domain.Authentication{
				Type:  domain.AuthenticationBearer,
				Token: keyValue(block.Lines, "token"),
			}
		case block.Name == "auth:basic":
			request.Authentication = &domain.Authentication{
				Type:     domain.AuthenticationBasic,
				Username: keyValue(block.Lines, "username"),
				Password: keyValue(block.Lines, "password"),
			}
		case block.Name == "auth:apikey":
			location := keyValue(block.Lines, "in")
			if location != "query" {
				location = "header"
			}
			request.Authentication = &domain.Authentication{
				Type:     domain.AuthenticationAPIKey,
				Key:      keyValue(block.Lines, "key"),
				Value:    keyValue(block.Lines, "value"),
				Location: location,
			}
		case block.Name == "auth:none":
			request.Authentication = &domain.Authentication{Type: domain.AuthenticationNone}
		case block.Name == "docs":
			request.Documentation = strings.TrimSpace(strings.Join(block.Lines, "\n"))
		case strings.HasPrefix(block.Name, "body:"):
			bodyType := brunoBodyType(strings.TrimPrefix(block.Name, "body:"))
			request.Body = &domain.Body{Type: bodyType}
			if request.Body.IsForm() {
				for _, entry := range brunoKeyValues(block.Lines) {
					request.Body.Fields = append(request.Body.Fields, domain.Parameter{Name: entry.key, Value: entry.value, Enabled: !entry.disabled})
				}
			} else {
				request.Body.Content = strings.TrimRight(strings.Join(block.Lines, "\n"), "\n")
			}
		}
	}

	if strings.TrimSpace(request.URL) == "" {
		return domain.Request{}, fmt.Errorf("bruno: request %q has no URL", request.Name)
	}

	if strings.TrimSpace(request.Name) == "" {
		request.Name = fallbackName
	}

	return request, nil
}

func parseBrunoCollectionFile(content string, fallbackName string) (Collection, error) {
	collection := Collection{
		Name:      fallbackName,
		Variables: map[string]string{},
	}

	for _, block := range parseBrunoBlocks(content) {
		switch block.Name {
		case "meta":
			if name := keyValue(block.Lines, "name"); name != "" {
				collection.Name = name
			}
		case "vars:collection":
			for _, entry := range brunoKeyValues(block.Lines) {
				if !entry.disabled {
					collection.Variables[entry.key] = entry.value
				}
			}
		case "docs":
			collection.Documentation = strings.TrimSpace(strings.Join(block.Lines, "\n"))
		case "auth:bearer":
			collection.Authentication = &domain.Authentication{
				Type:  domain.AuthenticationBearer,
				Token: keyValue(block.Lines, "token"),
			}
		case "auth:basic":
			collection.Authentication = &domain.Authentication{
				Type:     domain.AuthenticationBasic,
				Username: keyValue(block.Lines, "username"),
				Password: keyValue(block.Lines, "password"),
			}
		case "auth:apikey":
			location := keyValue(block.Lines, "in")
			if location != "query" {
				location = "header"
			}
			collection.Authentication = &domain.Authentication{
				Type:     domain.AuthenticationAPIKey,
				Key:      keyValue(block.Lines, "key"),
				Value:    keyValue(block.Lines, "value"),
				Location: location,
			}
		case "auth:none":
			collection.Authentication = &domain.Authentication{Type: domain.AuthenticationNone}
		}
	}

	if strings.TrimSpace(collection.Name) == "" {
		collection.Name = fallbackName
	}

	return collection, nil
}

func brunoRequestName(content string, fallbackName string) string {
	for _, block := range parseBrunoBlocks(content) {
		if block.Name == "meta" {
			if name := keyValue(block.Lines, "name"); name != "" {
				return name
			}
		}
	}
	return fallbackName
}

func parseBrunoBlocks(content string) []brunoBlock {
	lines := strings.Split(content, "\n")
	blocks := make([]brunoBlock, 0)

	for index := 0; index < len(lines); index += 1 {
		trimmed := strings.TrimSpace(lines[index])
		if !strings.HasSuffix(trimmed, "{") || strings.HasPrefix(trimmed, "}") {
			continue
		}

		name := strings.TrimSpace(strings.TrimSuffix(trimmed, "{"))
		if name == "" {
			continue
		}

		block := brunoBlock{Name: name}
		depth := 1

		for index += 1; index < len(lines); index += 1 {
			current := lines[index]
			if strings.TrimSpace(current) == "}" && depth == 1 {
				break
			}

			depth += strings.Count(current, "{") - strings.Count(current, "}")
			block.Lines = append(block.Lines, current)

			if depth <= 0 {
				break
			}
		}

		blocks = append(blocks, block)
	}

	return blocks
}

type brunoKeyValue struct {
	key      string
	value    string
	disabled bool
}

func brunoKeyValues(lines []string) []brunoKeyValue {
	entries := make([]brunoKeyValue, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		disabled := strings.HasPrefix(trimmed, "~")
		if disabled {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "~"))
		}

		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}

		key = unquote(strings.TrimSpace(key))
		if key == "" {
			continue
		}

		entries = append(entries, brunoKeyValue{
			key:      key,
			value:    unquote(strings.TrimSpace(value)),
			disabled: disabled,
		})
	}
	return entries
}

func keyValue(lines []string, key string) string {
	for _, entry := range brunoKeyValues(lines) {
		if entry.key == key {
			return entry.value
		}
	}
	return ""
}

func unquote(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return value[1 : len(value)-1]
	}
	return value
}

func brunoBodyType(name string) domain.BodyType {
	switch strings.ToLower(name) {
	case "json":
		return domain.BodyJSON
	case "xml":
		return domain.BodyXML
	case "text":
		return domain.BodyText
	case "form-urlencoded":
		return domain.BodyFormURLEncoded
	case "multipart":
		return domain.BodyMultipart
	case "binary", "file":
		return domain.BodyBinary
	default:
		return domain.BodyText
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
