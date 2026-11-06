// Package assistant connects Awake to command line coding agents such as
// Claude Code and OpenCode. It proposes the resolution of git conflicts and
// provides the chat that lets an agent change a workspace.
package assistant

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	binaryName     = "claude"
	openCodeName   = "opencode"
	versionTimeout = 10 * time.Second
	mergeTimeout   = 3 * time.Minute
	summaryPrefix  = "SUMMARY:"
)

// ErrNotInstalled is returned when an agent command cannot be found.
var ErrNotInstalled = errors.New("assistant: agent command is not installed")

// Status tells whether Claude Code can be used.
type Status struct {
	Installed bool
	Version   string
	Path      string
}

// Suggestion is a merged version of a conflicted file proposed by Claude.
type Suggestion struct {
	Content string
	// Summary explains in one sentence how the two versions were combined.
	Summary string
}

// runner executes an agent command; tests replace it.
type runner func(ctx context.Context, binary string, directory string, input string, arguments ...string) (string, error)

// Service runs the supported command line agents in non interactive mode.
type Service struct {
	run            runner
	lookup         func() (string, error)
	lookupOpenCode func() (string, error)
}

// NewService returns a service using the agent commands of this computer.
func NewService() *Service {
	return &Service{
		run:            runCommand,
		lookup:         findBinary,
		lookupOpenCode: findOpenCodeBinary,
	}
}

// Status reports whether Claude Code is installed, and its version.
func (service *Service) Status() Status {
	binary, err := service.lookup()
	if err != nil {
		return Status{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	output, err := service.run(ctx, binary, "", "", "--version")
	if err != nil {
		return Status{}
	}
	return Status{Installed: true, Version: strings.TrimSpace(output), Path: binary}
}

// SuggestMerge asks Claude to combine both sides of a conflicted file. Claude
// runs without any tool: it only reads the versions given in the prompt and
// can neither read nor change files, nor run commands.
func (service *Service) SuggestMerge(workspaceRoot string, path string, base string, local string, incoming string) (Suggestion, error) {
	binary, err := service.lookup()
	if err != nil {
		return Suggestion{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), mergeTimeout)
	defer cancel()

	output, err := service.run(ctx, binary, workspaceRoot, mergePrompt(path, base, local, incoming),
		"--print",
		"--output-format", "text",
		"--tools", "",
		"--no-session-persistence",
	)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Suggestion{}, fmt.Errorf("assistant: Claude did not answer within %s", mergeTimeout)
		}
		return Suggestion{}, err
	}
	return parseSuggestion(output)
}

func mergePrompt(path string, base string, local string, incoming string) string {
	var prompt strings.Builder
	prompt.WriteString(`You resolve a git merge conflict in Awake, an API client that stores each request, collection and environment as a YAML file.

Combine the two versions of the file below into one version that keeps the intent of both sides. When both sides changed the same value differently and the intent cannot be combined, prefer "yours" and mention it in the summary. Keep the YAML valid and keep the formatting style of the file.

Answer with exactly this layout and nothing else:
SUMMARY: <one sentence describing how the versions were combined>
<the complete merged file content>

Do not wrap the file in a code block. Do not add comments to the file.

`)
	fmt.Fprintf(&prompt, "File: %s\n\n", path)
	writeVersion(&prompt, "Common ancestor (base)", base)
	writeVersion(&prompt, "Yours (local changes)", local)
	writeVersion(&prompt, "Theirs (incoming changes)", incoming)
	return prompt.String()
}

func writeVersion(prompt *strings.Builder, title string, content string) {
	fmt.Fprintf(prompt, "===== %s =====\n%s\n===== end =====\n\n", title, strings.TrimRight(content, "\n"))
}

// parseSuggestion reads the summary line and the merged content, tolerating
// a code block around the content.
func parseSuggestion(output string) (Suggestion, error) {
	text := strings.TrimSpace(output)
	suggestion := Suggestion{}
	if strings.HasPrefix(text, summaryPrefix) {
		summary, rest, _ := strings.Cut(text, "\n")
		suggestion.Summary = strings.TrimSpace(strings.TrimPrefix(summary, summaryPrefix))
		text = rest
	}

	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if _, rest, found := strings.Cut(text, "\n"); found {
			text = rest
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Suggestion{}, errors.New("assistant: Claude returned no merged content")
	}
	suggestion.Content = text + "\n"
	return suggestion, nil
}

// findBinary looks for claude in the PATH, then where installers put it: a
// desktop application on macOS does not inherit the PATH of the shell.
func findBinary() (string, error) {
	home, _ := os.UserHomeDir()
	return findCommand(binaryName, []string{
		filepath.Join(home, ".local", "bin", binaryName),
		filepath.Join(home, ".claude", "local", binaryName),
		"/opt/homebrew/bin/" + binaryName,
		"/usr/local/bin/" + binaryName,
	})
}

// findOpenCodeBinary looks for opencode in the PATH, then in its installer
// directory.
func findOpenCodeBinary() (string, error) {
	home, _ := os.UserHomeDir()
	return findCommand(openCodeName, []string{
		filepath.Join(home, ".opencode", "bin", openCodeName),
		"/opt/homebrew/bin/" + openCodeName,
		"/usr/local/bin/" + openCodeName,
	})
}

func findCommand(name string, candidates []string) (string, error) {
	if binary, err := exec.LookPath(name); err == nil {
		return binary, nil
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", ErrNotInstalled
}

func runCommand(ctx context.Context, binary string, directory string, input string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Dir = directory
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", &commandError{
			binary: binary,
			err:    err,
			stdout: stdout.String(),
			stderr: stderr.String(),
		}
	}
	return stdout.String(), nil
}

// commandError keeps the output of a failed command so a caller can replace
// the raw stream with a readable message.
type commandError struct {
	binary string
	err    error
	stdout string
	stderr string
}

func (failure *commandError) Error() string {
	name := filepath.Base(failure.binary)
	message := strings.TrimSpace(failure.stderr)
	if message == "" {
		message = strings.TrimSpace(failure.stdout)
	}
	if message == "" {
		return fmt.Sprintf("assistant: %s failed: %v", name, failure.err)
	}
	return fmt.Sprintf("assistant: %s failed: %s", name, message)
}

func (failure *commandError) Unwrap() error {
	return failure.err
}
