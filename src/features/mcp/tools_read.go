package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/awake-client/awake/src/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultListLimit    = 200
	maxListLimit        = 1000
	defaultHistoryLimit = 20
	maxHistoryLimit     = 200
	// historyBodyLimit bounds the bodies of a history entry.
	historyBodyLimit = 64 * 1024
)

type listRequestsInput struct {
	Path  string `json:"path,omitempty" jsonschema:"collection or folder to list, such as collections/billing; empty lists the whole workspace"`
	Query string `json:"query,omitempty" jsonschema:"words that must all appear in the request name, path or method; searches requests only"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of entries, 200 by default"`
}

type entryOutput struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Kind   string `json:"kind" jsonschema:"collection, folder or request"`
	Method string `json:"method,omitempty"`
}

type listRequestsOutput struct {
	Entries   []entryOutput `json:"entries"`
	Truncated bool          `json:"truncated" jsonschema:"true when the limit cut the list"`
}

type pathInput struct {
	Path string `json:"path" jsonschema:"request path, such as collections/billing/invoices/list.yaml"`
}

type inheritedOutput struct {
	Type string `json:"type"`
	From string `json:"from" jsonschema:"collection or folder defining it"`
}

type requestOutput struct {
	Path    string         `json:"path"`
	Request map[string]any `json:"request" jsonschema:"the request, with the fields of its YAML file"`
	// InheritedAuthentication is set when the request inherits it.
	InheritedAuthentication *inheritedOutput `json:"inheritedAuthentication,omitempty" jsonschema:"authentication received from a folder or collection, when the request inherits it"`
}

type environmentOutput struct {
	FileName string `json:"fileName" jsonschema:"name to pass as environment"`
	Name     string `json:"name"`
	Scope    string `json:"scope" jsonschema:"empty for a global environment, else the collection or folder"`
	Private  bool   `json:"private"`
}

type listEnvironmentsOutput struct {
	Environments []environmentOutput `json:"environments"`
	Default      string              `json:"default" jsonschema:"environment used when a call names none"`
}

type variablesInput struct {
	RequestPath string  `json:"requestPath" jsonschema:"request whose variables to list"`
	Environment *string `json:"environment,omitempty" jsonschema:"environment file name; omitted uses the default, empty uses none"`
}

type variableOutput struct {
	Name   string `json:"name"`
	Value  string `json:"value" jsonschema:"value, or ••• for a secret or a credential such as a password or token"`
	Source string `json:"source" jsonschema:"layer defining it, such as collection environment"`
}

type variablesOutput struct {
	Environment string           `json:"environment"`
	Variables   []variableOutput `json:"variables"`
}

type historyInput struct {
	RequestPath string `json:"requestPath,omitempty" jsonschema:"only the history of this request"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum number of entries, 20 by default"`
}

type historyEntryOutput struct {
	ID          int64  `json:"id"`
	RequestPath string `json:"requestPath"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	StatusCode  int    `json:"statusCode"`
	DurationMs  int64  `json:"durationMs"`
	Environment string `json:"environment"`
	ExecutedAt  string `json:"executedAt"`
}

type historyOutput struct {
	Entries []historyEntryOutput `json:"entries"`
}

type historyEntryInput struct {
	ID int64 `json:"id" jsonschema:"history entry identifier, from list_history"`
}

type exchangeOutput struct {
	RequestHeaders        map[string][]string `json:"requestHeaders"`
	RequestBody           string              `json:"requestBody"`
	ResponseHeaders       map[string][]string `json:"responseHeaders"`
	ResponseBody          string              `json:"responseBody"`
	ResponseBodyEncoding  string              `json:"responseBodyEncoding"`
	ResponseBodyTruncated bool                `json:"responseBodyTruncated"`
}

type historyDetailOutput struct {
	historyEntryOutput
	Exchange *exchangeOutput `json:"exchange,omitempty" jsonschema:"what was sent and received; credentials are masked"`
}

func (tools *tools) registerRead(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_requests",
		Description: "List the collections, folders and requests of the workspace, or search requests by words.",
		Annotations: readOnly("List requests"),
	}, tools.listRequests)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_request",
		Description: "Read a request: method, URL, parameters, headers, authentication, body, scripts and documentation, with the fields of its YAML file.",
		Annotations: readOnly("Read a request"),
	}, tools.getRequest)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_environments",
		Description: "List the environments, global and per collection, and the default one.",
		Annotations: readOnly("List environments"),
	}, tools.listEnvironments)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_variables",
		Description: "List the variables a request can reference as {{name}} with an environment, and where each comes from. Secrets and credentials (passwords, tokens, keys) are masked.",
		Annotations: readOnly("List variables"),
	}, tools.getVariables)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_history",
		Description: "List the latest requests sent from the workspace, newest first.",
		Annotations: readOnly("List history"),
	}, tools.listHistory)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_history_entry",
		Description: "Read one history entry with the headers and bodies exchanged. Credentials are masked.",
		Annotations: readOnly("Read a history entry"),
	}, tools.getHistoryEntry)
}

func (tools *tools) listRequests(_ context.Context, _ *sdk.CallToolRequest, input listRequestsInput) (*sdk.CallToolResult, listRequestsOutput, error) {
	limit := clampLimit(input.Limit, defaultListLimit, maxListLimit)

	if strings.TrimSpace(input.Query) != "" {
		entries, err := tools.workspace.SearchRequests(input.Query, limit+1)
		if err != nil {
			return nil, listRequestsOutput{}, err
		}
		prefix := strings.TrimSuffix(input.Path, "/")
		output := listRequestsOutput{Entries: []entryOutput{}}
		for _, entry := range entries {
			if prefix != "" && !strings.HasPrefix(entry.Path, prefix+"/") {
				continue
			}
			if len(output.Entries) == limit {
				output.Truncated = true
				break
			}
			output.Entries = append(output.Entries, toEntryOutput(entry))
		}
		return nil, output, nil
	}

	// Walk the tree breadth first, so a limit keeps the upper levels.
	output := listRequestsOutput{Entries: []entryOutput{}}
	queue := []string{strings.TrimSuffix(input.Path, "/")}
	for len(queue) > 0 {
		directory := queue[0]
		queue = queue[1:]
		entries, err := tools.workspace.List(directory)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, listRequestsOutput{}, fmt.Errorf("%s does not exist; call list_requests without a path to see the collections", directory)
			}
			return nil, listRequestsOutput{}, err
		}
		for _, entry := range entries {
			if len(output.Entries) == limit {
				output.Truncated = true
				return nil, output, nil
			}
			output.Entries = append(output.Entries, toEntryOutput(entry))
			if entry.Kind != KindRequest && entry.HasChildren {
				queue = append(queue, entry.Path)
			}
		}
	}
	return nil, output, nil
}

func (tools *tools) getRequest(_ context.Context, _ *sdk.CallToolRequest, input pathInput) (*sdk.CallToolResult, requestOutput, error) {
	request, err := tools.workspace.LoadRequest(input.Path)
	if err != nil {
		return nil, requestOutput{}, describeEntryError(input.Path, err)
	}
	object, err := requestToObject(request)
	if err != nil {
		return nil, requestOutput{}, err
	}

	output := requestOutput{Path: input.Path, Request: object}
	if request.Authentication == nil || request.Authentication.Type == domain.AuthenticationInherit {
		if inherited, err := tools.workspace.InheritedAuthentication(input.Path); err == nil && inherited.From != "" {
			output.InheritedAuthentication = &inheritedOutput{Type: inherited.Type, From: inherited.From}
		}
	}
	return nil, output, nil
}

func (tools *tools) listEnvironments(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, listEnvironmentsOutput, error) {
	environments, err := tools.workspace.Environments()
	if err != nil {
		return nil, listEnvironmentsOutput{}, err
	}
	output := listEnvironmentsOutput{
		Environments: make([]environmentOutput, 0, len(environments)),
		Default:      tools.workspace.DefaultEnvironment(),
	}
	for _, environment := range environments {
		output.Environments = append(output.Environments, environmentOutput{
			FileName: environment.FileName,
			Name:     environment.Name,
			Scope:    environment.Scope,
			Private:  environment.Private,
		})
	}
	return nil, output, nil
}

func (tools *tools) getVariables(_ context.Context, _ *sdk.CallToolRequest, input variablesInput) (*sdk.CallToolResult, variablesOutput, error) {
	environment := tools.environmentOrDefault(input.Environment)
	variables, err := tools.sender.Variables(input.RequestPath, environment)
	if err != nil {
		return nil, variablesOutput{}, describeEntryError(input.RequestPath, err)
	}
	output := variablesOutput{Environment: environment, Variables: make([]variableOutput, 0, len(variables))}
	for _, variable := range variables {
		value := variable.Value
		if variable.Secret || looksSensitive(variable.Name) {
			value = maskedValue
		}
		output.Variables = append(output.Variables, variableOutput{Name: variable.Name, Value: value, Source: variable.Source})
	}
	return nil, output, nil
}

func (tools *tools) listHistory(_ context.Context, _ *sdk.CallToolRequest, input historyInput) (*sdk.CallToolResult, historyOutput, error) {
	entries, err := tools.history.List(input.RequestPath, clampLimit(input.Limit, defaultHistoryLimit, maxHistoryLimit))
	if err != nil {
		return nil, historyOutput{}, err
	}
	output := historyOutput{Entries: make([]historyEntryOutput, 0, len(entries))}
	for _, entry := range entries {
		output.Entries = append(output.Entries, historyEntryOutput{
			ID:          entry.ID,
			RequestPath: entry.RequestPath,
			Method:      entry.Method,
			URL:         entry.URL,
			StatusCode:  entry.StatusCode,
			DurationMs:  entry.DurationMilliseconds,
			Environment: entry.Environment,
			ExecutedAt:  entry.ExecutedAt,
		})
	}
	return nil, output, nil
}

func (tools *tools) getHistoryEntry(_ context.Context, _ *sdk.CallToolRequest, input historyEntryInput) (*sdk.CallToolResult, historyDetailOutput, error) {
	entry, err := tools.history.Entry(input.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, historyDetailOutput{}, fmt.Errorf("history entry %d does not exist; call list_history", input.ID)
		}
		return nil, historyDetailOutput{}, err
	}

	output := historyDetailOutput{historyEntryOutput: historyEntryOutput{
		ID:          entry.ID,
		RequestPath: entry.RequestPath,
		Method:      entry.Method,
		URL:         entry.URL,
		StatusCode:  entry.StatusCode,
		DurationMs:  entry.DurationMilliseconds,
		Environment: entry.Environment,
		ExecutedAt:  entry.ExecutedAt,
	}}
	if exchange := entry.Exchange; exchange != nil {
		responseBody, cut := truncateText(exchange.ResponseBody, historyBodyLimit)
		requestBody, _ := truncateText(exchange.RequestBody, historyBodyLimit)
		output.Exchange = &exchangeOutput{
			RequestHeaders:        exchange.RequestHeaders,
			RequestBody:           requestBody,
			ResponseHeaders:       exchange.ResponseHeaders,
			ResponseBody:          responseBody,
			ResponseBodyEncoding:  exchange.ResponseEncoding,
			ResponseBodyTruncated: exchange.ResponseTruncated || cut,
		}
	}
	return nil, output, nil
}

// sensitiveNameParts mark variables holding credentials even when they are
// not declared as secrets, such as a password typed in a private environment.
var sensitiveNameParts = []string{"password", "passwd", "secret", "token", "apikey", "api_key", "api-key", "privatekey", "private_key", "credential"}

// looksSensitive reports a variable whose name suggests a credential.
func looksSensitive(name string) bool {
	lower := strings.ToLower(name)
	for _, part := range sensitiveNameParts {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

func toEntryOutput(entry Entry) entryOutput {
	return entryOutput{Path: entry.Path, Name: entry.Name, Kind: entry.Kind, Method: entry.Method}
}

func clampLimit(limit int, fallback int, maximum int) int {
	if limit <= 0 {
		return fallback
	}
	return min(limit, maximum)
}

// describeEntryError turns a missing entry into a hint the agent can act on.
func describeEntryError(path string, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return fmt.Errorf("%s does not exist; call list_requests to find the right path", path)
	case errors.Is(err, ErrExists):
		return fmt.Errorf("%s already exists; use update_request to change it", path)
	default:
		return err
	}
}
