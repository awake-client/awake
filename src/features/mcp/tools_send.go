package mcp

import (
	"context"
	"unicode/utf8"

	"github.com/awake-client/awake/src/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultBodyBytes = 64 * 1024
	// MaxBodyBytes is the largest body preview a call can ask for.
	MaxBodyBytes = 256 * 1024
	maskedValue  = "•••"
)

type sendInput struct {
	Path         string  `json:"path" jsonschema:"request to send, such as collections/billing/invoices/list.yaml"`
	Environment  *string `json:"environment,omitempty" jsonschema:"environment file name; omitted uses the default, empty uses none"`
	MaxBodyBytes int     `json:"maxBodyBytes,omitempty" jsonschema:"largest response body returned, 65536 by default and 262144 at most"`
}

type scriptTestOutput struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}

type scriptOutput struct {
	Logs  []string           `json:"logs,omitempty"`
	Tests []scriptTestOutput `json:"tests,omitempty"`
	Error string             `json:"error,omitempty"`
	// EnvironmentChanges lists the variables the script stored, by name only.
	EnvironmentChanges []string `json:"environmentChanges,omitempty" jsonschema:"names of the variables the script stored in the environment"`
}

type scriptsOutput struct {
	PreRequest   *scriptOutput `json:"preRequest,omitempty"`
	PostResponse *scriptOutput `json:"postResponse,omitempty"`
}

type sendOutput struct {
	Environment   string              `json:"environment"`
	StatusCode    int                 `json:"statusCode"`
	Status        string              `json:"status"`
	Headers       map[string][]string `json:"headers"`
	ContentType   string              `json:"contentType"`
	Body          *string             `json:"body" jsonschema:"response body as text, or null for a binary body"`
	BodyTruncated bool                `json:"bodyTruncated" jsonschema:"true when only the start of the body is returned"`
	SizeBytes     int64               `json:"sizeBytes"`
	DurationMs    int64               `json:"durationMs"`
	Scripts       scriptsOutput       `json:"scripts"`
}

func (tools *tools) registerSend(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{
		Name: "send_request",
		Description: "Send a saved request with an environment, exactly as the Send button of Awake: variables, " +
			"inherited authentication, OAuth 2 tokens and scripts apply, and the exchange is added to the history. " +
			"Returns the status, headers and the start of the body.",
		Annotations: &sdk.ToolAnnotations{Title: "Send a request", OpenWorldHint: boolPointer(true)},
	}, tools.sendRequest)
}

func (tools *tools) sendRequest(_ context.Context, _ *sdk.CallToolRequest, input sendInput) (*sdk.CallToolResult, sendOutput, error) {
	environment := tools.environmentOrDefault(input.Environment)
	limit := clampLimit(input.MaxBodyBytes, defaultBodyBytes, MaxBodyBytes)

	response, err := tools.sender.Send(input.Path, environment, limit)
	if err != nil {
		return nil, sendOutput{}, describeEntryError(input.Path, err)
	}

	output := sendOutput{
		Environment:   environment,
		StatusCode:    response.StatusCode,
		Status:        response.Status,
		Headers:       response.Headers,
		ContentType:   response.ContentType,
		BodyTruncated: response.Truncated,
		SizeBytes:     response.Size,
		DurationMs:    response.DurationMs,
		Scripts: scriptsOutput{
			PreRequest:   toScriptOutput(response.PreRequest),
			PostResponse: toScriptOutput(response.PostResponse),
		},
	}
	if !response.Binary {
		body, cut := truncateText(response.Body, limit)
		output.Body = &body
		output.BodyTruncated = output.BodyTruncated || cut
	}
	return nil, output, nil
}

func toScriptOutput(outcome domain.ScriptOutcome) *scriptOutput {
	if len(outcome.Logs) == 0 && len(outcome.Tests) == 0 && outcome.Error == "" && len(outcome.EnvironmentChanges) == 0 {
		return nil
	}
	output := &scriptOutput{Logs: outcome.Logs, Error: outcome.Error}
	for _, test := range outcome.Tests {
		output.Tests = append(output.Tests, scriptTestOutput{Name: test.Name, Passed: test.Passed, Error: test.Error})
	}
	// Values are left out: a script often stores a token.
	for _, change := range outcome.EnvironmentChanges {
		output.EnvironmentChanges = append(output.EnvironmentChanges, change.Name)
	}
	return output
}

// truncateText keeps at most limit bytes of a text without splitting a
// character, and reports whether it cut it.
func truncateText(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut], true
}
