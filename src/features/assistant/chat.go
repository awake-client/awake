package assistant

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/awake-client/awake/src/shared/files"
)

const (
	// AgentClaude is the name of the Claude Code command line agent.
	AgentClaude = "claude"
	// AgentOpenCode is the name of the OpenCode command line agent.
	AgentOpenCode = "opencode"

	agentFileName = "AGENT.md"

	chatTimeout             = 10 * time.Minute
	claudeChatTools         = "Read,Glob,Grep,Edit,Write,Bash(mv *),Bash(mkdir *)"
	maxHistoryMessages      = 24
	maxHistoryMessageLength = 4000
)

var (
	// ErrWorkspaceNotFound is returned when the workspace directory does not
	// exist.
	ErrWorkspaceNotFound = errors.New("assistant: workspace not found")
	// ErrUnknownAgent is returned when the requested agent is not supported.
	ErrUnknownAgent = errors.New("assistant: unknown agent")
	// ErrEmptyMessage is returned when a chat message is blank.
	ErrEmptyMessage = errors.New("assistant: empty message")
)

//go:embed AGENT.md
var agentInstructions string

// AgentInstructions documents the workspace conventions for coding agents,
// as written to AGENT.md at the root of a workspace.
func AgentInstructions() string {
	return agentInstructions
}

// AgentStatus tells whether a command line agent can be used.
type AgentStatus struct {
	Name      string
	Label     string
	Installed bool
	Version   string
	Path      string
}

// ChatMessage is one turn of a conversation sent to an agent.
type ChatMessage struct {
	// Role is "user" or "assistant".
	Role string
	// Content is the plain text of the turn.
	Content string
}

// ChatReply is the answer of an agent to one chat message.
type ChatReply struct {
	Content string
	// Agent is the name of the agent that answered.
	Agent string
	// CreatedAgentFile reports whether this run installed AGENT.md.
	CreatedAgentFile bool
}

type openCodeEvent struct {
	Type string `json:"type"`
	Part struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"part"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Agents reports the supported agents and whether they are installed.
func (service *Service) Agents() []AgentStatus {
	return []AgentStatus{
		service.describeAgent(AgentClaude, "Claude Code", service.lookup),
		service.describeAgent(AgentOpenCode, "OpenCode", service.lookupOpenCode),
	}
}

// OpenCodeModels lists the models OpenCode can use, so the chat can offer
// them. An empty list means OpenCode is missing or could not answer.
func (service *Service) OpenCodeModels() []string {
	if service.lookupOpenCode == nil {
		return nil
	}
	binary, err := service.lookupOpenCode()
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()

	output, err := service.run(ctx, binary, "", "", "models")
	if err != nil {
		return nil
	}

	models := make([]string, 0, 32)
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			models = append(models, line)
		}
	}
	return models
}

// Chat runs one turn of a conversation with an agent. The agent works at the
// workspace root, can read and edit its files, and receives the previous turns
// so a conversation can continue without keeping server-side state. An
// AGENT.md file describing the workspace conventions is installed when missing.
// The model is only used by OpenCode; an empty value keeps its configured one.
func (service *Service) Chat(workspaceRoot string, agent string, model string, message string, history []ChatMessage) (ChatReply, error) {
	if err := validateChatWorkspace(workspaceRoot); err != nil {
		return ChatReply{}, err
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return ChatReply{}, ErrEmptyMessage
	}

	binary, err := service.lookupAgent(agent)
	if err != nil {
		return ChatReply{}, err
	}

	created, err := ensureAgentFile(workspaceRoot)
	if err != nil {
		return ChatReply{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), chatTimeout)
	defer cancel()

	output, err := service.run(
		ctx,
		binary,
		workspaceRoot,
		chatPrompt(workspaceRoot, message, history),
		chatArguments(agent, model)...,
	)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ChatReply{}, fmt.Errorf("assistant: the agent did not answer within %s", chatTimeout)
		}
		if agent == AgentOpenCode {
			if failureMessage, ok := openCodeFailure(err, model); ok {
				return ChatReply{}, errors.New(failureMessage)
			}
		}
		return ChatReply{}, err
	}

	content := strings.TrimSpace(output)
	if agent == AgentOpenCode {
		content, err = parseOpenCodeOutput(output)
		if err != nil {
			return ChatReply{}, err
		}
	}
	if content == "" {
		return ChatReply{}, errors.New("assistant: the agent returned no answer")
	}

	return ChatReply{
		Content:          content,
		Agent:            agent,
		CreatedAgentFile: created,
	}, nil
}

func (service *Service) describeAgent(name string, label string, lookup func() (string, error)) AgentStatus {
	status := AgentStatus{Name: name, Label: label}
	if lookup == nil {
		return status
	}

	binary, err := lookup()
	if err != nil {
		return status
	}

	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()

	output, err := service.run(ctx, binary, "", "", "--version")
	if err != nil {
		return status
	}

	status.Installed = true
	status.Version = cleanVersion(label, output)
	status.Path = binary
	return status
}

// cleanVersion keeps the first line of `--version`, which agents decorate
// differently: Claude answers "2.1.319 (Claude Code)" and OpenCode
// "opencode v2.2.5".
func cleanVersion(label string, output string) string {
	version := strings.TrimSpace(output)
	if line, _, found := strings.Cut(version, "\n"); found {
		version = strings.TrimSpace(line)
	}
	if index := strings.Index(version, " ("); index > 0 {
		version = version[:index]
	}
	name := strings.ToLower(strings.TrimSpace(label))
	if strings.HasPrefix(strings.ToLower(version), name) {
		version = version[len(name):]
	}
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return strings.TrimSpace(output)
	}
	return version
}

func (service *Service) lookupAgent(agent string) (string, error) {
	switch agent {
	case AgentClaude:
		if service.lookup == nil {
			return "", ErrNotInstalled
		}
		return service.lookup()
	case AgentOpenCode:
		if service.lookupOpenCode == nil {
			return "", ErrNotInstalled
		}
		return service.lookupOpenCode()
	default:
		return "", ErrUnknownAgent
	}
}

func chatArguments(agent string, model string) []string {
	switch agent {
	case AgentClaude:
		return []string{
			"--print",
			"--output-format", "text",
			"--permission-mode", "acceptEdits",
			"--allowedTools", claudeChatTools,
			"--no-session-persistence",
		}
	case AgentOpenCode:
		arguments := []string{
			"run",
			"--format", "json",
			"--auto",
		}
		if model = strings.TrimSpace(model); model != "" {
			arguments = append(arguments, "--model", model)
		}
		return arguments
	default:
		return nil
	}
}

func validateChatWorkspace(workspaceRoot string) error {
	if strings.TrimSpace(workspaceRoot) == "" {
		return ErrWorkspaceNotFound
	}
	info, err := os.Stat(workspaceRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrWorkspaceNotFound
		}
		return fmt.Errorf("assistant: inspect workspace: %w", err)
	}
	if !info.IsDir() {
		return ErrWorkspaceNotFound
	}
	return nil
}

func ensureAgentFile(workspaceRoot string) (bool, error) {
	target := filepath.Join(workspaceRoot, agentFileName)
	info, err := os.Stat(target)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("assistant: %s is a directory", agentFileName)
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("assistant: inspect %s: %w", agentFileName, err)
	}

	if err := files.WriteAtomically(target, []byte(agentInstructions)); err != nil {
		return false, fmt.Errorf("assistant: write %s: %w", agentFileName, err)
	}
	return true, nil
}

func chatPrompt(workspaceRoot string, message string, history []ChatMessage) string {
	var prompt strings.Builder
	prompt.WriteString("You are the assistant inside Awake, a git-native API client. The user talks to you from the Assistant chat.\n\n")
	fmt.Fprintf(&prompt, "Workspace root: %s\n", workspaceRoot)
	prompt.WriteString(`Collections, requests and environments are YAML files inside this workspace. Use your file tools to make the requested changes directly in the workspace. Stay inside the workspace. Read AGENT.md at the root before editing; it describes the file format and the conventions.

Do not commit, push or delete files unless the user asks. When you are done, answer with a short chat message, in the language the user writes in.

`)
	writeChatHistory(&prompt, history)
	fmt.Fprintf(&prompt, "User: %s\n", message)
	return prompt.String()
}

func writeChatHistory(prompt *strings.Builder, history []ChatMessage) {
	if len(history) == 0 {
		return
	}

	start := 0
	if len(history) > maxHistoryMessages {
		start = len(history) - maxHistoryMessages
	}

	prompt.WriteString("Conversation so far:\n")
	for _, item := range history[start:] {
		content := clipHistory(strings.TrimSpace(item.Content))
		if content == "" {
			continue
		}
		role := "User"
		if strings.EqualFold(strings.TrimSpace(item.Role), "assistant") {
			role = "Assistant"
		}
		fmt.Fprintf(prompt, "%s: %s\n", role, content)
	}
	prompt.WriteString("\n")
}

func clipHistory(content string) string {
	characters := []rune(content)
	if len(characters) <= maxHistoryMessageLength {
		return content
	}
	return string(characters[:maxHistoryMessageLength]) + "…"
}

// openCodeFailure replaces the raw JSON event stream printed by a failed
// OpenCode run with the error message it carries, so the chat does not show
// the whole stream.
func openCodeFailure(err error, model string) (string, bool) {
	var failure *commandError
	if !errors.As(err, &failure) {
		return "", false
	}
	_, parseErr := parseOpenCodeOutput(failure.stdout)
	if parseErr == nil {
		return "", false
	}

	message := parseErr.Error()
	if strings.TrimSpace(model) == "" && mentionsToolUse(message) {
		message += " The model OpenCode uses by default cannot edit files here: choose another model in the Assistant."
	}
	return message, true
}

func mentionsToolUse(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "tool use") || strings.Contains(lower, "tool_use") || strings.Contains(lower, "tool-use")
}

// parseOpenCodeOutput reads the JSON event stream printed by
// `opencode run --format json` and keeps only the assistant text.
func parseOpenCodeOutput(output string) (string, error) {
	var text strings.Builder
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event openCodeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if event.Type == "error" {
			if event.Error.Message != "" {
				return "", fmt.Errorf("assistant: %s", event.Error.Message)
			}
			return "", errors.New("assistant: the agent reported an error")
		}
		if event.Part.Type == "text" {
			text.WriteString(event.Part.Text)
		}
	}
	return strings.TrimSpace(text.String()), nil
}
