package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/assistant"
	"github.com/awake-client/awake/src/features/collections"
	"github.com/awake-client/awake/src/features/environments"
	"github.com/awake-client/awake/src/features/history"
	"github.com/awake-client/awake/src/features/httpclient"
	"github.com/awake-client/awake/src/features/mcp"
	"github.com/awake-client/awake/src/features/updates"
	"github.com/awake-client/awake/src/features/workspace"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// workspaceFileName marks the root of a workspace.
const workspaceFileName = "awake.yaml"

// RunMCP serves one workspace to a coding agent over stdin and stdout, for
// `awake mcp [--workspace <directory>] [--read-only]`. No window opens.
func RunMCP(arguments []string) error {
	flags := flag.NewFlagSet("awake mcp", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspaceFlag := flags.String("workspace", "", "workspace directory; by default the closest parent of the current directory holding awake.yaml")
	readOnly := flags.Bool("read-only", false, "only read and send requests, never create or change files")
	if err := flags.Parse(arguments); err != nil {
		return err
	}

	root, err := findWorkspaceRoot(*workspaceFlag)
	if err != nil {
		return err
	}
	configuration, err := workspace.NewRepository(root).Load()
	if err != nil {
		return fmt.Errorf("open workspace %s: %w", root, err)
	}

	enabled, err := mcp.Enabled(root)
	if err != nil {
		return err
	}
	if !enabled {
		return mcp.ErrDisabled
	}

	services, err := newServices()
	if err != nil {
		return err
	}

	server := newMCPServer(root, configuration, services, *readOnly)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = server.Run(ctx, &sdk.StdioTransport{})
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func newMCPServer(root string, configuration workspace.Configuration, services *services, readOnly bool) *sdk.Server {
	return mcp.NewServer(
		mcpWorkspace{root: root, configuration: configuration, collections: services.Collections, environments: services.Environments},
		mcpSender{root: root, client: services.HTTPClient},
		mcpHistory{root: root, history: services.History},
		mcp.Options{
			ReadOnly: readOnly,
			Version:  updates.Version,
			Enabled: func() bool {
				enabled, err := mcp.Enabled(root)
				return err == nil && enabled
			},
		},
	)
}

// findWorkspaceRoot returns the given directory, or the closest parent of
// the current directory that holds awake.yaml.
func findWorkspaceRoot(directory string) (string, error) {
	if directory != "" {
		absolute, err := filepath.Abs(directory)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(absolute, workspaceFileName)); err != nil {
			return "", fmt.Errorf("%s is not an Awake workspace: %s is missing", absolute, workspaceFileName)
		}
		return absolute, nil
	}

	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, workspaceFileName)); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no %s found from the current directory; pass --workspace <directory>", workspaceFileName)
		}
		current = parent
	}
}

// mcpError maps the errors of the features to those of the MCP ports.
func mcpError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, collections.ErrEntryNotFound), errors.Is(err, history.ErrEntryNotFound), errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("%w: %v", mcp.ErrNotFound, err)
	case errors.Is(err, collections.ErrEntryExists):
		return fmt.Errorf("%w: %v", mcp.ErrExists, err)
	default:
		return err
	}
}

// mcpWorkspace adapts the collections and environments features.
type mcpWorkspace struct {
	root          string
	configuration workspace.Configuration
	collections   *collections.Service
	environments  *environments.Service
}

func (adapter mcpWorkspace) List(path string) ([]mcp.Entry, error) {
	entries, err := adapter.collections.List(adapter.root, path)
	return toMCPEntries(entries), mcpError(err)
}

func (adapter mcpWorkspace) SearchRequests(query string, limit int) ([]mcp.Entry, error) {
	entries, err := adapter.collections.SearchRequests(adapter.root, query, limit)
	return toMCPEntries(entries), mcpError(err)
}

func (adapter mcpWorkspace) LoadRequest(path string) (domain.Request, error) {
	request, err := adapter.collections.LoadRequest(adapter.root, path)
	return request, mcpError(err)
}

func (adapter mcpWorkspace) InheritedAuthentication(path string) (mcp.InheritedAuthentication, error) {
	inherited, err := adapter.collections.LoadInheritedAuthentication(adapter.root, path)
	if err != nil {
		return mcp.InheritedAuthentication{}, mcpError(err)
	}
	result := mcp.InheritedAuthentication{From: inherited.SourcePath}
	if inherited.Authentication != nil {
		result.Type = string(inherited.Authentication.Type)
	}
	return result, nil
}

func (adapter mcpWorkspace) CreateRequest(path string, request domain.Request) error {
	return mcpError(adapter.collections.CreateRequest(adapter.root, path, request))
}

func (adapter mcpWorkspace) UpdateRequest(path string, request domain.Request) error {
	return mcpError(adapter.collections.UpdateRequest(adapter.root, path, request))
}

func (adapter mcpWorkspace) CreateFolder(path string, name string) error {
	return mcpError(adapter.collections.CreateFolder(adapter.root, path, name))
}

func (adapter mcpWorkspace) CreateCollection(path string, collection domain.Collection) error {
	return mcpError(adapter.collections.CreateCollection(adapter.root, path, collection))
}

func (adapter mcpWorkspace) Environments() ([]mcp.Environment, error) {
	summaries, err := adapter.environments.ListAll(adapter.root)
	if err != nil {
		return nil, err
	}
	result := make([]mcp.Environment, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, mcp.Environment{
			FileName: summary.FileName,
			Name:     summary.Name,
			Scope:    summary.Scope,
			Private:  summary.Private,
		})
	}
	return result, nil
}

func (adapter mcpWorkspace) DefaultEnvironment() string {
	return adapter.configuration.DefaultEnvironment
}

func (mcpWorkspace) Guide() string {
	return assistant.AgentInstructions()
}

func toMCPEntries(entries []collections.Entry) []mcp.Entry {
	result := make([]mcp.Entry, 0, len(entries))
	for _, entry := range entries {
		kind := mcp.KindRequest
		switch entry.Kind {
		case collections.KindCollection:
			kind = mcp.KindCollection
		case collections.KindDirectory:
			kind = mcp.KindFolder
		}
		result = append(result, mcp.Entry{
			Path:        entry.Path,
			Name:        entry.Name,
			Kind:        kind,
			Method:      entry.Method,
			HasChildren: entry.HasChildren,
		})
	}
	return result
}

// mcpSender adapts the HTTP client.
type mcpSender struct {
	root   string
	client *httpclient.Service
}

func (adapter mcpSender) Send(path string, environment string, maxBodyBytes int) (mcp.Response, error) {
	response, err := adapter.client.Send(adapter.root, path, environment)
	if err != nil {
		if errors.Is(err, httpclient.ErrOAuth2AuthorizationRequired) {
			return mcp.Response{}, errors.New("this request uses OAuth 2 with a browser login: open it in Awake and click Authorize in browser first")
		}
		return mcp.Response{}, mcpError(err)
	}
	// The body stays on disk for the interface; the preview is enough here.
	defer func() { _ = adapter.client.Close(response.ID) }()

	result := mcp.Response{
		StatusCode:   response.StatusCode,
		Status:       response.Status,
		Headers:      response.Headers,
		ContentType:  response.ContentType,
		Binary:       response.Encoding == "base64",
		Truncated:    response.Truncated,
		Size:         response.Size,
		DurationMs:   response.DurationMilliseconds,
		PreRequest:   response.PreRequestOutcome,
		PostResponse: response.PostResponseOutcome,
	}
	if !result.Binary {
		result.Body = response.Preview
		if len(result.Body) > maxBodyBytes {
			result.Truncated = true
		}
	}
	return result, nil
}

func (adapter mcpSender) Variables(path string, environment string) ([]mcp.Variable, error) {
	variables, err := adapter.client.ResolveVariables(adapter.root, path, environment)
	if err != nil {
		return nil, mcpError(err)
	}
	result := make([]mcp.Variable, 0, len(variables))
	for _, variable := range variables {
		result = append(result, mcp.Variable{
			Name:   variable.Name,
			Value:  variable.Value,
			Source: variable.Source,
			Secret: variable.Secret,
		})
	}
	return result, nil
}

// mcpHistory adapts the history feature.
type mcpHistory struct {
	root    string
	history *history.Service
}

func (adapter mcpHistory) List(requestPath string, limit int) ([]domain.HistoryEntry, error) {
	if requestPath != "" {
		entries, err := adapter.history.ListForRequest(adapter.root, requestPath, limit, 0)
		return entries, mcpError(err)
	}
	entries, err := adapter.history.List(adapter.root, limit, 0)
	return entries, mcpError(err)
}

func (adapter mcpHistory) Entry(id int64) (domain.HistoryEntry, error) {
	entry, err := adapter.history.Entry(adapter.root, id)
	return entry, mcpError(err)
}
