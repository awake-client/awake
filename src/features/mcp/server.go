package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// GuideURI is the resource documenting the workspace file formats.
const GuideURI = "awake://guide"

const instructions = `Awake is an API client. This server works on one Awake workspace.

Paths are relative to the workspace and use "/": a collection is
"collections/<name>" (or "local/<name>" when private), a folder adds
"/<folder>", and a request is a ".yaml" file inside them.

Typical flow: list_requests, then get_request to read one, send_request to run
it with an environment, and get_request then update_request to change it.
Requests use the same fields as the request files; read the awake://guide
resource for the format. Secret values never leave Awake: they show as "•••".`

// Options configures the server.
type Options struct {
	// ReadOnly leaves out the tools that create or change files.
	ReadOnly bool
	// Version is reported to the agent.
	Version string
	// Enabled is checked before every tool call and resource read, so turning
	// the server off in Awake takes effect without restarting the agent. Nil
	// means always enabled.
	Enabled func() bool
}

// NewServer returns an MCP server exposing the workspace to agents.
func NewServer(workspace Workspace, sender Sender, history History, options Options) *sdk.Server {
	version := options.Version
	if version == "" {
		version = "dev"
	}
	server := sdk.NewServer(
		&sdk.Implementation{Name: "awake", Title: "Awake", Version: version},
		&sdk.ServerOptions{Instructions: instructions},
	)

	if options.Enabled != nil {
		server.AddReceivingMiddleware(disabledGuard(options.Enabled))
	}

	tools := &tools{workspace: workspace, sender: sender, history: history}
	tools.registerRead(server)
	tools.registerSend(server)
	if !options.ReadOnly {
		tools.registerWrite(server)
	}

	server.AddResource(&sdk.Resource{
		URI:         GuideURI,
		Name:        "guide",
		Title:       "Awake workspace guide",
		Description: "File formats of requests, collections and environments.",
		MIMEType:    "text/markdown",
	}, func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI:      GuideURI,
			MIMEType: "text/markdown",
			Text:     workspace.Guide(),
		}}}, nil
	})

	return server
}

// disabledGuard refuses tool calls and resource reads while the server is
// turned off. A tool call fails as a tool error, so the agent shows why.
func disabledGuard(enabled func() bool) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
			if enabled() {
				return next(ctx, method, request)
			}
			switch method {
			case "tools/call":
				return &sdk.CallToolResult{
					IsError: true,
					Content: []sdk.Content{&sdk.TextContent{Text: ErrDisabled.Error()}},
				}, nil
			case "resources/read":
				return nil, ErrDisabled
			default:
				return next(ctx, method, request)
			}
		}
	}
}

// tools holds the ports the tool handlers use.
type tools struct {
	workspace Workspace
	sender    Sender
	history   History
}

func boolPointer(value bool) *bool {
	return &value
}

// readOnly describes a tool that only reads the workspace.
func readOnly(title string) *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: boolPointer(false)}
}

// environmentOrDefault applies the documented rule: no environment given
// selects the default one, an empty string selects none.
func (tools *tools) environmentOrDefault(environment *string) string {
	if environment == nil {
		return tools.workspace.DefaultEnvironment()
	}
	return *environment
}
