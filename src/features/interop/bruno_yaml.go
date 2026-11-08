package interop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/awake-client/awake/src/domain"
	"go.yaml.in/yaml/v3"
)

const (
	brunoYAMLCollectionFileName   = "opencollection.yml"
	brunoYAMLFolderFileName       = "folder.yml"
	brunoYAMLEnvironmentDir       = "environments"
	brunoYAMLExtension            = ".yml"
	brunoYAMLAlternativeExtension = ".yaml"
)

// brunoYAMLCollectionFile is the root document of an OpenCollection YAML
// collection.
type brunoYAMLCollectionFile struct {
	OpenCollection string                    `yaml:"opencollection"`
	Info           brunoYAMLInfo             `yaml:"info"`
	Request        *brunoYAMLRequestDefaults `yaml:"request"`
	Docs           brunoYAMLDescription      `yaml:"docs"`
}

type brunoYAMLInfo struct {
	Name string   `yaml:"name"`
	Type string   `yaml:"type"`
	Seq  *float64 `yaml:"seq"`
}

type brunoYAMLRequestDefaults struct {
	Auth      *brunoYAMLAuth      `yaml:"auth"`
	Variables []brunoYAMLVariable `yaml:"variables"`
}

type brunoYAMLFolderFile struct {
	Info    brunoYAMLInfo             `yaml:"info"`
	Request *brunoYAMLRequestDefaults `yaml:"request"`
	Docs    brunoYAMLDescription      `yaml:"docs"`
}

type brunoYAMLRequestFile struct {
	Info    brunoYAMLInfo            `yaml:"info"`
	HTTP    brunoYAMLHTTP            `yaml:"http"`
	Runtime *brunoYAMLRequestRuntime `yaml:"runtime"`
	Docs    brunoYAMLDescription     `yaml:"docs"`
}

type brunoYAMLHTTP struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers []brunoYAMLHeader `yaml:"headers"`
	Params  []brunoYAMLParam  `yaml:"params"`
	Body    *brunoYAMLBody    `yaml:"body"`
	Auth    *brunoYAMLAuth    `yaml:"auth"`
}

type brunoYAMLRequestRuntime struct {
	Auth *brunoYAMLAuth `yaml:"auth"`
}

type brunoYAMLHeader struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Disabled bool   `yaml:"disabled"`
}

type brunoYAMLParam struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Type     string `yaml:"type"`
	Disabled bool   `yaml:"disabled"`
}

type brunoYAMLBody struct {
	Type string    `yaml:"type"`
	Data yaml.Node `yaml:"data"`
}

type brunoYAMLFormField struct {
	Name     string    `yaml:"name"`
	Value    yaml.Node `yaml:"value"`
	Disabled bool      `yaml:"disabled"`
}

// brunoYAMLAuth accepts both the scalar form ("inherit", "none") and the
// mapping form ({type: bearer, token: ...}) used by OpenCollection.
type brunoYAMLAuth struct {
	Type      string `yaml:"type"`
	Token     string `yaml:"token"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Key       string `yaml:"key"`
	Value     string `yaml:"value"`
	Placement string `yaml:"placement"`
	In        string `yaml:"in"`
	Flow      string `yaml:"flow"`

	AuthorizationURL string `yaml:"authorizationUrl"`
	AccessTokenURL   string `yaml:"accessTokenUrl"`
	CallbackURL      string `yaml:"callbackUrl"`
	ClientID         string `yaml:"clientId"`
	ClientSecret     string `yaml:"clientSecret"`
	Scope            string `yaml:"scope"`
	Audience         string `yaml:"audience"`
}

type brunoYAMLVariable struct {
	Name     string         `yaml:"name"`
	Value    brunoYAMLValue `yaml:"value"`
	Disabled bool           `yaml:"disabled"`
}

// brunoYAMLValue reads plain, typed and variant variable values.
type brunoYAMLValue string

// brunoYAMLDescription reads the scalar and typed {content, type} forms.
type brunoYAMLDescription string

// readBrunoYAMLCollection parses an unbundled OpenCollection collection.
func readBrunoYAMLCollection(root string) (Collection, error) {
	collection := Collection{
		Name:      filepath.Base(root),
		Variables: map[string]string{},
	}

	configurationPath := filepath.Join(root, brunoYAMLCollectionFileName)
	content, err := os.ReadFile(configurationPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Collection{}, fmt.Errorf("read %s: %w", configurationPath, err)
		}
		// Some collections use the .yaml extension for the root file.
		configurationPath = filepath.Join(root, "opencollection.yaml")
		content, err = os.ReadFile(configurationPath)
		if err != nil {
			return Collection{}, fmt.Errorf("read %s: %w", configurationPath, err)
		}
	}

	metadata := brunoYAMLCollectionFile{}
	if err := yaml.Unmarshal(content, &metadata); err != nil {
		return Collection{}, fmt.Errorf("read %s: %w", configurationPath, err)
	}

	if name := strings.TrimSpace(metadata.Info.Name); name != "" {
		collection.Name = name
	}
	collection.Documentation = strings.TrimSpace(string(metadata.Docs))
	if metadata.Request != nil {
		collection.Authentication = metadata.Request.Auth.authentication()
		for _, variable := range metadata.Request.Variables {
			if variable.Disabled || variable.Name == "" {
				continue
			}
			collection.Variables[variable.Name] = string(variable.Value)
		}
	}

	requests, folders, err := readBrunoYAMLItems(root, true)
	if err != nil {
		return Collection{}, fmt.Errorf("read bruno collection: %w", err)
	}
	collection.Requests = requests
	collection.Folders = folders

	if len(collection.Requests) == 0 && len(collection.Folders) == 0 {
		return Collection{}, ErrEmptyCollection
	}

	return collection, nil
}

func readBrunoYAMLItems(directory string, isRoot bool) ([]Request, []Folder, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, err
	}

	sequencedRequests := make([]sequencedBrunoYAMLRequest, 0)
	sequencedFolders := make([]sequencedBrunoYAMLFolder, 0)

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		entryPath := filepath.Join(directory, name)
		if entry.IsDir() {
			if isRoot && name == brunoYAMLEnvironmentDir {
				continue
			}
			if name == "node_modules" {
				continue
			}

			requests, folders, err := readBrunoYAMLItems(entryPath, false)
			if err != nil {
				return nil, nil, err
			}

			folder := brunoYAMLFolderFile{}
			if content, err := os.ReadFile(filepath.Join(entryPath, brunoYAMLFolderFileName)); err == nil {
				if err := yaml.Unmarshal(content, &folder); err != nil {
					return nil, nil, fmt.Errorf("read %s: %w", filepath.Join(entryPath, brunoYAMLFolderFileName), err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, nil, err
			}

			folderName := strings.TrimSpace(folder.Info.Name)
			if folderName == "" {
				folderName = name
			}

			sequencedFolders = append(sequencedFolders, sequencedBrunoYAMLFolder{
				seq:  folder.Info.Seq,
				name: folderName,
				folder: Folder{
					Name:          folderName,
					Documentation: strings.TrimSpace(string(folder.Docs)),
					Requests:      requests,
					Folders:       folders,
				},
			})
			continue
		}

		if !isBrunoYAMLFile(name) || isBrunoYAMLReservedFile(name) {
			continue
		}

		content, err := os.ReadFile(entryPath)
		if err != nil {
			return nil, nil, err
		}

		fallbackName := strings.TrimSuffix(name, filepath.Ext(name))
		request, ok, err := parseBrunoYAMLRequest(content, fallbackName)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", name, err)
		}
		if !ok {
			continue
		}

		sequencedRequests = append(sequencedRequests, sequencedBrunoYAMLRequest{
			seq:     brunoYAMLRequestSequence(content),
			name:    request.Name,
			request: Request{Name: request.Name, Request: request},
		})
	}

	sort.SliceStable(sequencedRequests, func(i, j int) bool {
		return brunoYAMLSequenceLess(
			sequencedRequests[i].seq,
			sequencedRequests[j].seq,
			sequencedRequests[i].name,
			sequencedRequests[j].name,
		)
	})
	sort.SliceStable(sequencedFolders, func(i, j int) bool {
		return brunoYAMLSequenceLess(
			sequencedFolders[i].seq,
			sequencedFolders[j].seq,
			sequencedFolders[i].name,
			sequencedFolders[j].name,
		)
	})

	requests := make([]Request, 0, len(sequencedRequests))
	for _, entry := range sequencedRequests {
		requests = append(requests, entry.request)
	}

	folders := make([]Folder, 0, len(sequencedFolders))
	for _, entry := range sequencedFolders {
		folders = append(folders, entry.folder)
	}

	return requests, folders, nil
}

type sequencedBrunoYAMLRequest struct {
	seq     *float64
	name    string
	request Request
}

type sequencedBrunoYAMLFolder struct {
	seq    *float64
	name   string
	folder Folder
}

func brunoYAMLSequenceLess(a *float64, b *float64, aName string, bName string) bool {
	switch {
	case a != nil && b != nil:
		if *a != *b {
			return *a < *b
		}
	case a != nil:
		return true
	case b != nil:
		return false
	}
	return aName < bName
}

func parseBrunoYAMLRequest(content []byte, fallbackName string) (domain.Request, bool, error) {
	document := brunoYAMLRequestFile{}
	if err := yaml.Unmarshal(content, &document); err != nil {
		return domain.Request{}, false, err
	}

	requestType := strings.ToLower(strings.TrimSpace(document.Info.Type))
	if requestType != "" && requestType != "http" {
		return domain.Request{}, false, nil
	}

	name := strings.TrimSpace(document.Info.Name)
	if name == "" {
		name = fallbackName
	}

	request := domain.Request{
		Name:           name,
		Method:         strings.ToUpper(strings.TrimSpace(document.HTTP.Method)),
		URL:            document.HTTP.URL,
		Authentication: &domain.Authentication{Type: domain.AuthenticationInherit},
		Body:           &domain.Body{Type: domain.BodyNone},
		Documentation:  strings.TrimSpace(string(document.Docs)),
	}
	if request.Method == "" {
		request.Method = domain.MethodGet
	}

	auth := document.HTTP.Auth
	if auth == nil && document.Runtime != nil {
		auth = document.Runtime.Auth
	}
	if authentication := auth.authentication(); authentication != nil {
		request.Authentication = authentication
	}

	if strings.TrimSpace(request.URL) == "" {
		return domain.Request{}, false, fmt.Errorf("bruno: request %q has no URL", request.Name)
	}

	for _, header := range document.HTTP.Headers {
		request.Headers = append(request.Headers, domain.Header{
			Name:    header.Name,
			Value:   header.Value,
			Enabled: !header.Disabled,
		})
	}

	for _, parameter := range document.HTTP.Params {
		if strings.EqualFold(parameter.Type, "path") {
			request.PathParameters = append(request.PathParameters, domain.PathParameter{Name: parameter.Name, Value: parameter.Value})
			continue
		}
		if !strings.EqualFold(parameter.Type, "query") && parameter.Type != "" {
			continue
		}
		request.Parameters = append(request.Parameters, domain.Parameter{
			Name:    parameter.Name,
			Value:   parameter.Value,
			Enabled: !parameter.Disabled,
		})
	}

	request.Body = brunoYAMLBodyValue(document.HTTP.Body)

	return request, true, nil
}

func brunoYAMLRequestSequence(content []byte) *float64 {
	document := brunoYAMLRequestFile{}
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil
	}
	return document.Info.Seq
}

func brunoYAMLBodyValue(body *brunoYAMLBody) *domain.Body {
	if body == nil || strings.TrimSpace(body.Type) == "" {
		return &domain.Body{Type: domain.BodyNone}
	}

	bodyType := brunoBodyType(body.Type)
	switch bodyType {
	case domain.BodyFormURLEncoded, domain.BodyMultipart:
		return &domain.Body{Type: bodyType, Fields: brunoYAMLFormFields(&body.Data)}
	case domain.BodyBinary:
		return &domain.Body{Type: bodyType, Content: brunoYAMLNodeString(&body.Data)}
	default:
		return &domain.Body{Type: bodyType, Content: brunoYAMLNodeString(&body.Data)}
	}
}

func brunoYAMLFormFields(node *yaml.Node) []domain.Parameter {
	var fields []brunoYAMLFormField
	if err := node.Decode(&fields); err != nil {
		return nil
	}

	result := make([]domain.Parameter, 0, len(fields))
	for _, field := range fields {
		if field.Name == "" {
			continue
		}
		result = append(result, domain.Parameter{Name: field.Name, Value: brunoYAMLNodeString(&field.Value), Enabled: !field.Disabled})
	}
	return result
}

func brunoYAMLNodeString(node *yaml.Node) string {
	if node == nil || node.Kind == 0 {
		return ""
	}
	if node.Kind == yaml.ScalarNode {
		return node.Value
	}

	var value string
	if err := node.Decode(&value); err == nil {
		return value
	}
	return strings.TrimSpace(node.Value)
}

func isBrunoYAMLFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, brunoYAMLExtension) ||
		strings.HasSuffix(lower, brunoYAMLAlternativeExtension)
}

func isBrunoYAMLReservedFile(name string) bool {
	lower := strings.ToLower(name)
	return lower == brunoYAMLCollectionFileName ||
		lower == "opencollection.yaml" ||
		lower == brunoYAMLFolderFileName ||
		lower == "folder.yaml"
}

func (description *brunoYAMLDescription) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*description = brunoYAMLDescription(node.Value)
	case yaml.MappingNode:
		var typed struct {
			Content string `yaml:"content"`
		}
		if err := node.Decode(&typed); err != nil {
			return err
		}
		*description = brunoYAMLDescription(typed.Content)
	}
	return nil
}

func (value *brunoYAMLValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*value = brunoYAMLValue(node.Value)
	case yaml.MappingNode:
		var typed struct {
			Data string `yaml:"data"`
		}
		if err := node.Decode(&typed); err != nil {
			return err
		}
		*value = brunoYAMLValue(typed.Data)
	case yaml.SequenceNode:
		var variants []struct {
			Selected bool           `yaml:"selected"`
			Value    brunoYAMLValue `yaml:"value"`
		}
		if err := node.Decode(&variants); err != nil {
			return err
		}
		for _, variant := range variants {
			if variant.Selected {
				*value = variant.Value
				return nil
			}
		}
		if len(variants) > 0 {
			*value = variants[0].Value
		}
	}
	return nil
}

func (auth *brunoYAMLAuth) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		auth.Type = strings.ToLower(strings.TrimSpace(node.Value))
		return nil
	}

	type plain brunoYAMLAuth
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*auth = brunoYAMLAuth(decoded)
	if auth.Type == "" {
		auth.Type = strings.ToLower(strings.TrimSpace(auth.Flow))
	}
	return nil
}

func (auth *brunoYAMLAuth) authentication() *domain.Authentication {
	if auth == nil {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(auth.Type)) {
	case "", "inherit":
		return &domain.Authentication{Type: domain.AuthenticationInherit}
	case "none":
		return &domain.Authentication{Type: domain.AuthenticationNone}
	case "bearer":
		return &domain.Authentication{Type: domain.AuthenticationBearer, Token: auth.Token}
	case "basic":
		return &domain.Authentication{
			Type:     domain.AuthenticationBasic,
			Username: auth.Username,
			Password: auth.Password,
		}
	case "apikey":
		location := auth.Placement
		if location == "" {
			location = auth.In
		}
		if location != "query" {
			location = "header"
		}
		return &domain.Authentication{
			Type:     domain.AuthenticationAPIKey,
			Key:      auth.Key,
			Value:    auth.Value,
			Location: location,
		}
	case "oauth2":
		return &domain.Authentication{
			Type:             domain.AuthenticationOAuth2,
			GrantType:        brunoYAMLOAuth2Grant(auth.Flow),
			AuthorizationURL: auth.AuthorizationURL,
			TokenURL:         auth.AccessTokenURL,
			ClientID:         auth.ClientID,
			ClientSecret:     auth.ClientSecret,
			Scopes:           brunoYAMLScopes(auth.Scope),
			RedirectURL:      auth.CallbackURL,
			Audience:         auth.Audience,
		}
	default:
		return nil
	}
}

func brunoYAMLOAuth2Grant(flow string) domain.OAuth2GrantType {
	switch strings.ToLower(strings.TrimSpace(flow)) {
	case "client_credentials":
		return domain.OAuth2GrantClientCredentials
	case "password", "resource_owner_password_credentials":
		return domain.OAuth2GrantPassword
	default:
		return domain.OAuth2GrantAuthorizationCode
	}
}

func brunoYAMLScopes(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Fields(value)
}
