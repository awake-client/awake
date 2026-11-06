package assistant

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatRunsClaudeWithEditingTools(t *testing.T) {
	workspaceRoot := t.TempDir()

	var (
		directory string
		input     string
		arguments []string
	)
	service := &Service{
		lookup: func() (string, error) { return "/bin/claude", nil },
		run: func(
			_ context.Context,
			_ string,
			runDirectory string,
			runInput string,
			runArguments ...string,
		) (string, error) {
			directory = runDirectory
			input = runInput
			arguments = runArguments
			return "  Renamed two requests.  \n", nil
		},
	}

	history := []ChatMessage{
		{Role: "user", Content: "Where are the requests?"},
		{Role: "assistant", Content: "Under collections/shop."},
	}
	reply, err := service.Chat(workspaceRoot, AgentClaude, "", "Rename the unclear ones.", history)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if !reply.CreatedAgentFile {
		t.Error("the first chat must install AGENT.md")
	}
	if reply.Agent != AgentClaude {
		t.Errorf("unexpected agent %q", reply.Agent)
	}
	if reply.Content != "Renamed two requests." {
		t.Errorf("the answer must be trimmed, got %q", reply.Content)
	}

	instructions, err := os.ReadFile(filepath.Join(workspaceRoot, agentFileName))
	if err != nil {
		t.Fatalf("read AGENT.md: %v", err)
	}
	if !strings.Contains(string(instructions), "## Working in the workspace") {
		t.Error("AGENT.md must contain the workspace instructions")
	}

	if directory != workspaceRoot {
		t.Errorf("the agent must run at the workspace root, got %q", directory)
	}

	joined := strings.Join(arguments, " ")
	for _, expected := range []string{
		"--print",
		"--output-format text",
		"--permission-mode acceptEdits",
		"--allowedTools Read,Glob,Grep,Edit,Write,Bash(mv *),Bash(mkdir *)",
		"--no-session-persistence",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("the command must contain %q, got %q", expected, joined)
		}
	}

	for _, expected := range []string{
		workspaceRoot,
		"Rename the unclear ones.",
		"Where are the requests?",
		"Under collections/shop.",
		agentFileName,
	} {
		if !strings.Contains(input, expected) {
			t.Errorf("the prompt must contain %q, got %q", expected, input)
		}
	}
}

func TestChatRunsOpenCodeAndParsesItsTextEvents(t *testing.T) {
	workspaceRoot := t.TempDir()

	var arguments []string
	service := &Service{
		lookupOpenCode: func() (string, error) { return "/bin/opencode", nil },
		run: func(_ context.Context, _ string, _ string, _ string, runArguments ...string) (string, error) {
			arguments = runArguments
			return strings.Join([]string{
				`{"type":"step_start","part":{"type":"step-start"}}`,
				`{"type":"text","part":{"type":"text","text":"Here is "}}`,
				`{"type":"text","part":{"type":"text","text":"the answer."}}`,
				`{"type":"step_finish","part":{"type":"step-finish","reason":"stop"}}`,
			}, "\n"), nil
		},
	}

	reply, err := service.Chat(workspaceRoot, AgentOpenCode, "opencode/test-model", "What is in this workspace?", nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if reply.Content != "Here is the answer." {
		t.Errorf("unexpected answer %q", reply.Content)
	}
	if reply.Agent != AgentOpenCode {
		t.Errorf("unexpected agent %q", reply.Agent)
	}

	joined := strings.Join(arguments, " ")
	if joined != "run --format json --auto --model opencode/test-model" {
		t.Errorf("unexpected OpenCode arguments %q", joined)
	}
}

func TestChatUnwrapsAnOpenCodeFailure(t *testing.T) {
	service := &Service{
		lookupOpenCode: func() (string, error) { return "/bin/opencode", nil },
		run: func(context.Context, string, string, string, ...string) (string, error) {
			return "", &commandError{
				binary: "/bin/opencode",
				err:    errors.New("exit status 1"),
				stdout: `{"type":"step_start","part":{"type":"step-start"}}
{"type":"error","error":{"message":"No endpoints found that support tool use."}}`,
			}
		},
	}

	_, err := service.Chat(t.TempDir(), AgentOpenCode, "opencode/test-model", "Do something.", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Error() != "assistant: No endpoints found that support tool use." {
		t.Errorf("the raw stream must be replaced by the message, got %q", err)
	}
}

func TestChatHintsAtTheModelWhenToolUseIsUnsupported(t *testing.T) {
	service := &Service{
		lookupOpenCode: func() (string, error) { return "/bin/opencode", nil },
		run: func(context.Context, string, string, string, ...string) (string, error) {
			return "", &commandError{
				binary: "/bin/opencode",
				err:    errors.New("exit status 1"),
				stdout: `{"type":"error","error":{"message":"No endpoints found that support tool use."}}`,
			}
		},
	}

	_, err := service.Chat(t.TempDir(), AgentOpenCode, "", "Do something.", nil)
	if err == nil || !strings.Contains(err.Error(), "choose another model") {
		t.Fatalf("expected a model hint, got %v", err)
	}
}

func TestOpenCodeModelsListsTheAvailableModels(t *testing.T) {
	service := &Service{
		lookupOpenCode: func() (string, error) { return "/bin/opencode", nil },
		run: func(_ context.Context, _ string, _ string, _ string, arguments ...string) (string, error) {
			if strings.Join(arguments, " ") != "models" {
				t.Errorf("unexpected arguments %v", arguments)
			}
			return "opencode/gpt-5.5\n\nopencode/deepseek-v4.1-flash\n", nil
		},
	}

	models := service.OpenCodeModels()
	want := []string{"opencode/gpt-5.5", "opencode/deepseek-v4.1-flash"}
	if len(models) != len(want) {
		t.Fatalf("unexpected models %v", models)
	}
	for index, model := range want {
		if models[index] != model {
			t.Errorf("models[%d] = %q, want %q", index, models[index], model)
		}
	}
}

func TestCommandErrorNamesTheAgent(t *testing.T) {
	failure := &commandError{
		binary: "/usr/local/bin/opencode",
		err:    errors.New("exit status 1"),
		stderr: "boom",
	}
	if got := failure.Error(); got != "assistant: opencode failed: boom" {
		t.Errorf("unexpected message %q", got)
	}
}

func TestCleanVersion(t *testing.T) {
	cases := []struct {
		label  string
		output string
		want   string
	}{
		{"Claude Code", "2.1.319 (Claude Code)\n", "2.1.319"},
		{"OpenCode", "opencode v2.2.5\n", "2.2.5"},
		{"OpenCode", "2.2.5", "2.2.5"},
	}
	for _, testCase := range cases {
		if got := cleanVersion(testCase.label, testCase.output); got != testCase.want {
			t.Errorf("cleanVersion(%q, %q) = %q, want %q", testCase.label, testCase.output, got, testCase.want)
		}
	}
}

func TestChatReportsOpenCodeErrors(t *testing.T) {
	service := &Service{
		lookupOpenCode: func() (string, error) { return "/bin/opencode", nil },
		run: func(context.Context, string, string, string, ...string) (string, error) {
			return `{"type":"error","error":{"message":"No endpoints found that support tool use."}}`, nil
		},
	}

	_, err := service.Chat(t.TempDir(), AgentOpenCode, "", "Do something.", nil)
	if err == nil || !strings.Contains(err.Error(), "No endpoints found") {
		t.Fatalf("expected the OpenCode error, got %v", err)
	}
}

func TestChatKeepsAnExistingAgentFile(t *testing.T) {
	workspaceRoot := t.TempDir()
	target := filepath.Join(workspaceRoot, agentFileName)
	existing := []byte("# Team instructions\nDo not replace this file.\n")
	if err := os.WriteFile(target, existing, 0o644); err != nil {
		t.Fatalf("write AGENT.md: %v", err)
	}

	service := &Service{
		lookup: func() (string, error) { return "/bin/claude", nil },
		run: func(context.Context, string, string, string, ...string) (string, error) {
			return "Done.", nil
		},
	}

	reply, err := service.Chat(workspaceRoot, AgentClaude, "", "Hello.", nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if reply.CreatedAgentFile {
		t.Error("an existing AGENT.md must not be reported as created")
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read AGENT.md: %v", err)
	}
	if string(content) != string(existing) {
		t.Errorf("the existing AGENT.md was overwritten: %q", content)
	}
}

func TestChatRejectsInvalidRequests(t *testing.T) {
	service := &Service{
		lookup:         func() (string, error) { return "/bin/claude", nil },
		lookupOpenCode: func() (string, error) { return "/bin/opencode", nil },
		run: func(context.Context, string, string, string, ...string) (string, error) {
			t.Fatal("the agent must not run for an invalid request")
			return "", nil
		},
	}

	if _, err := service.Chat(t.TempDir(), "unknown", "", "Hello.", nil); !errors.Is(err, ErrUnknownAgent) {
		t.Errorf("expected ErrUnknownAgent, got %v", err)
	}
	if _, err := service.Chat(t.TempDir(), AgentClaude, "", "   ", nil); !errors.Is(err, ErrEmptyMessage) {
		t.Errorf("expected ErrEmptyMessage, got %v", err)
	}
	if _, err := service.Chat(filepath.Join(t.TempDir(), "missing"), AgentClaude, "", "Hello.", nil); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestChatRequiresAnInstalledAgent(t *testing.T) {
	workspaceRoot := t.TempDir()
	service := &Service{
		lookupOpenCode: func() (string, error) { return "", ErrNotInstalled },
		run: func(context.Context, string, string, string, ...string) (string, error) {
			t.Fatal("the agent must not run when the binary is missing")
			return "", nil
		},
	}

	if _, err := service.Chat(workspaceRoot, AgentOpenCode, "", "Hello.", nil); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected ErrNotInstalled, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, agentFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("AGENT.md must not be installed when the agent is missing")
	}
}

func TestAgentsReportsBothAgents(t *testing.T) {
	service := &Service{
		lookup:         func() (string, error) { return "/bin/claude", nil },
		lookupOpenCode: func() (string, error) { return "/bin/opencode", nil },
		run: func(_ context.Context, binary string, _ string, _ string, _ ...string) (string, error) {
			if binary == "/bin/claude" {
				return "2.5.0\n", nil
			}
			return "2.2.5\n", nil
		},
	}

	agents := service.Agents()
	if len(agents) != 2 {
		t.Fatalf("expected two agents, got %d", len(agents))
	}
	for _, agent := range agents {
		if !agent.Installed {
			t.Errorf("%s must be reported as installed", agent.Name)
		}
	}
	if agents[0].Name != AgentClaude || agents[0].Version != "2.5.0" {
		t.Errorf("unexpected Claude status %+v", agents[0])
	}
	if agents[1].Name != AgentOpenCode || agents[1].Version != "2.2.5" {
		t.Errorf("unexpected OpenCode status %+v", agents[1])
	}

	missing := &Service{
		lookup:         func() (string, error) { return "/bin/claude", nil },
		lookupOpenCode: func() (string, error) { return "", ErrNotInstalled },
		run: func(context.Context, string, string, string, ...string) (string, error) {
			return "2.5.0", nil
		},
	}
	if missing.Agents()[1].Installed {
		t.Error("a missing OpenCode binary must not be reported as installed")
	}
}

func TestChatPromptKeepsTheRecentHistory(t *testing.T) {
	history := make([]ChatMessage, 0, 30)
	for index := 0; index < 30; index++ {
		history = append(history, ChatMessage{
			Role:    "user",
			Content: "history-" + string(rune('a'+index)),
		})
	}

	prompt := chatPrompt("/workspace", "Last question.", history)
	if strings.Contains(prompt, "history-a:") {
		t.Error("the oldest turns must be dropped from the prompt")
	}
	if !strings.Contains(prompt, "history-") {
		t.Error("the recent turns must be kept in the prompt")
	}
	if !strings.Contains(prompt, "Last question.") {
		t.Error("the current message must be in the prompt")
	}
}
