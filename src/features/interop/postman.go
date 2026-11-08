package interop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/awake-client/awake/src/domain"
)

type postmanCollection struct {
	Info     postmanInfo       `json:"info"`
	Item     []postmanItem     `json:"item"`
	Variable []postmanVariable `json:"variable"`
	Auth     *postmanAuth      `json:"auth"`
}

type postmanInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type postmanItem struct {
	Name    string          `json:"name"`
	Item    []postmanItem   `json:"item"`
	Request json.RawMessage `json:"request"`
	Event   json.RawMessage `json:"event"`
}

type postmanVariable struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
	Type     string `json:"type"`
}

type postmanRequest struct {
	Method string          `json:"method"`
	Header []postmanHeader `json:"header"`
	URL    postmanURL      `json:"url"`
	Body   *postmanBody    `json:"body"`
	Auth   *postmanAuth    `json:"auth"`
}

type postmanHeader struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

type postmanURL struct {
	Raw      string              `json:"raw"`
	Protocol string              `json:"protocol"`
	Host     flexibleStringList  `json:"host"`
	Path     flexibleStringList  `json:"path"`
	Query    []postmanQueryEntry `json:"query"`
	// Variable gives the values of the :name segments of the path.
	Variable []postmanVariable `json:"variable"`
}

type postmanQueryEntry struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

type postmanBody struct {
	Mode       string             `json:"mode"`
	Raw        string             `json:"raw"`
	URLEncoded []postmanFormEntry `json:"urlencoded"`
	FormData   []postmanFormEntry `json:"formdata"`
	File       *postmanFile       `json:"file"`
	Options    *postmanOptions    `json:"options"`
}

type postmanFormEntry struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Type     string `json:"type"`
	Src      string `json:"src"`
	Disabled bool   `json:"disabled"`
}

type postmanFile struct {
	Src string `json:"src"`
}

type postmanOptions struct {
	Raw *postmanRawOptions `json:"raw"`
}

type postmanRawOptions struct {
	Language string `json:"language"`
}

type postmanAuth struct {
	Type   string             `json:"type"`
	Bearer []postmanAuthEntry `json:"bearer"`
	Basic  []postmanAuthEntry `json:"basic"`
	APIKey []postmanAuthEntry `json:"apikey"`
}

type postmanAuthEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type flexibleStringList []string

func (values *flexibleStringList) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*values = flexibleStringList{single}
		return nil
	}

	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*values = flexibleStringList(list)
	return nil
}

func (rawURL *postmanURL) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		rawURL.Raw = single
		return nil
	}

	type plain postmanURL
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*rawURL = postmanURL(decoded)
	return nil
}

func (request *postmanRequest) UnmarshalJSON(data []byte) error {
	var rawURL string
	if err := json.Unmarshal(data, &rawURL); err == nil {
		request.Method = domain.MethodGet
		request.URL.Raw = rawURL
		return nil
	}

	type plain postmanRequest
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*request = postmanRequest(decoded)
	return nil
}

// ReadPostman parses a Postman collection, version 2.0 or 2.1.
func ReadPostman(content []byte) ([]Collection, error) {
	var document postmanCollection
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("decode postman collection: %w", err)
	}
	if document.Info.Name == "" && len(document.Item) == 0 {
		return nil, ErrUnsupportedFormat
	}

	collection := Collection{
		Name:           document.Info.Name,
		Documentation:  document.Info.Description,
		Variables:      map[string]string{},
		Authentication: postmanAuthentication(document.Auth),
	}
	if strings.TrimSpace(collection.Name) == "" {
		collection.Name = "Imported collection"
	}

	for _, variable := range document.Variable {
		if variable.Disabled || variable.Key == "" {
			continue
		}
		collection.Variables[variable.Key] = variable.Value
	}

	collection.Requests, collection.Folders = readPostmanItems(document.Item)

	if len(collection.Requests) == 0 && len(collection.Folders) == 0 {
		return nil, ErrEmptyCollection
	}

	return []Collection{collection}, nil
}

func readPostmanItems(items []postmanItem) ([]Request, []Folder) {
	requests := make([]Request, 0, len(items))
	folders := make([]Folder, 0)

	for _, item := range items {
		if len(item.Item) > 0 {
			folder := Folder{Name: item.Name}
			folder.Requests, folder.Folders = readPostmanItems(item.Item)
			folders = append(folders, folder)
			continue
		}

		if len(item.Request) == 0 || string(item.Request) == "null" {
			continue
		}

		var request postmanRequest
		if err := json.Unmarshal(item.Request, &request); err != nil {
			continue
		}

		converted := convertPostmanRequest(item.Name, request)
		requests = append(requests, converted)
	}

	return requests, folders
}

func convertPostmanRequest(name string, request postmanRequest) Request {
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = domain.MethodGet
	}

	converted := domain.Request{
		Name:           name,
		Method:         method,
		URL:            postmanURLValue(request.URL),
		Authentication: postmanAuthentication(request.Auth),
		Body:           postmanBodyValue(request.Body),
	}

	for _, header := range request.Header {
		if header.Key == "" {
			continue
		}
		converted.Headers = append(converted.Headers, domain.Header{
			Name:    header.Key,
			Value:   header.Value,
			Enabled: !header.Disabled,
		})
	}

	for _, variable := range request.URL.Variable {
		if variable.Key != "" {
			converted.PathParameters = append(converted.PathParameters, domain.PathParameter{Name: variable.Key, Value: variable.Value})
		}
	}

	for _, query := range request.URL.Query {
		if query.Key == "" {
			continue
		}
		converted.Parameters = append(converted.Parameters, domain.Parameter{
			Name:    query.Key,
			Value:   query.Value,
			Enabled: !query.Disabled,
		})
	}

	return Request{Name: name, Request: converted}
}

func postmanURLValue(rawURL postmanURL) string {
	if rawURL.Raw != "" {
		return rawURL.Raw
	}

	host := strings.Join(rawURL.Host, ".")
	pathValue := strings.Join(rawURL.Path, "/")

	value := pathValue
	if host != "" {
		value = host + "/" + pathValue
	}
	if rawURL.Protocol != "" {
		value = rawURL.Protocol + "://" + value
	}

	return value
}

func postmanAuthentication(auth *postmanAuth) *domain.Authentication {
	if auth == nil || auth.Type == "" {
		return &domain.Authentication{Type: domain.AuthenticationInherit}
	}

	switch strings.ToLower(auth.Type) {
	case "bearer":
		return &domain.Authentication{
			Type:  domain.AuthenticationBearer,
			Token: authValue(auth.Bearer, "token"),
		}
	case "basic":
		return &domain.Authentication{
			Type:     domain.AuthenticationBasic,
			Username: authValue(auth.Basic, "username"),
			Password: authValue(auth.Basic, "password"),
		}
	case "apikey":
		location := authValue(auth.APIKey, "in")
		if location != "query" {
			location = "header"
		}
		return &domain.Authentication{
			Type:     domain.AuthenticationAPIKey,
			Key:      authValue(auth.APIKey, "key"),
			Value:    authValue(auth.APIKey, "value"),
			Location: location,
		}
	case "noauth":
		return &domain.Authentication{Type: domain.AuthenticationNone}
	default:
		return &domain.Authentication{Type: domain.AuthenticationInherit}
	}
}

func authValue(entries []postmanAuthEntry, key string) string {
	for _, entry := range entries {
		if entry.Key == key {
			return entry.Value
		}
	}
	return ""
}

func postmanBodyValue(body *postmanBody) *domain.Body {
	if body == nil || body.Mode == "" {
		return &domain.Body{Type: domain.BodyNone}
	}

	switch body.Mode {
	case "raw":
		bodyType := domain.BodyText
		if body.Options != nil && body.Options.Raw != nil {
			switch strings.ToLower(body.Options.Raw.Language) {
			case "json":
				bodyType = domain.BodyJSON
			case "xml":
				bodyType = domain.BodyXML
			}
		}
		return &domain.Body{Type: bodyType, Content: body.Raw}
	case "urlencoded":
		return &domain.Body{
			Type:   domain.BodyFormURLEncoded,
			Fields: postmanFormFields(body.URLEncoded),
		}
	case "formdata":
		return &domain.Body{
			Type:   domain.BodyMultipart,
			Fields: postmanFormFields(body.FormData),
		}
	case "file":
		if body.File != nil {
			return &domain.Body{Type: domain.BodyBinary, Content: body.File.Src}
		}
		return &domain.Body{Type: domain.BodyBinary}
	default:
		return &domain.Body{Type: domain.BodyNone}
	}
}

func postmanFormFields(entries []postmanFormEntry) []domain.Parameter {
	fields := make([]domain.Parameter, 0, len(entries))
	for _, entry := range entries {
		if entry.Key == "" {
			continue
		}
		fields = append(fields, domain.Parameter{Name: entry.Key, Value: entry.Value, Enabled: !entry.Disabled})
	}
	return fields
}
