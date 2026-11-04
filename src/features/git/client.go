// Package git wraps the system git command line interface. Awake uses the
// installed git binary instead of a library so that credential helpers, SSH
// configuration and three-way merges behave exactly like the user expects.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client runs git commands inside a single working directory.
type Client struct {
	workingDirectory string
}

// Open returns a client for the repository containing directory.
func Open(ctx context.Context, directory string) (*Client, error) {
	client := &Client{workingDirectory: directory}

	isRepository, err := client.IsRepository(ctx)
	if err != nil {
		return nil, err
	}
	if !isRepository {
		return nil, fmt.Errorf("git: %s is not a git repository", directory)
	}

	return client, nil
}

// Init creates a repository with the given initial branch.
func Init(ctx context.Context, directory string, initialBranch string) error {
	client := &Client{workingDirectory: directory}

	_, err := client.run(ctx, "init", "--initial-branch="+initialBranch)
	return err
}

// IsRepository reports whether the directory belongs to a git work tree.
func (client *Client) IsRepository(ctx context.Context) (bool, error) {
	output, err := client.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		var commandError *CommandError
		if errors.As(err, &commandError) {
			return false, nil
		}
		return false, err
	}

	return strings.TrimSpace(string(output)) == "true", nil
}

// Stage adds the given paths to the index.
func (client *Client) Stage(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}

	arguments := append([]string{"add", "--"}, paths...)
	_, err := client.run(ctx, arguments...)
	return err
}

// Commit creates a commit from the staged changes and returns its hash.
func (client *Client) Commit(ctx context.Context, message string) (string, error) {
	if _, err := client.run(ctx, "commit", "--message", message); err != nil {
		return "", err
	}

	hash, err := client.run(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(hash)), nil
}

// Pull fetches and merges the upstream branch. When the merge produces
// conflicts, the returned result lists them and no error is returned: the
// caller is expected to resolve the conflicts before continuing.
func (client *Client) Pull(ctx context.Context) (PullResult, error) {
	result := PullResult{}

	output, err := client.run(ctx, "pull", "--no-rebase", "--no-edit")
	if err != nil {
		status, statusError := client.Status(ctx)
		if statusError != nil {
			return result, err
		}

		result.Conflicts = status.ConflictedPaths()
		if len(result.Conflicts) == 0 {
			return result, err
		}
		return result, nil
	}

	result.AlreadyUpToDate = strings.Contains(string(output), "Already up to date")
	return result, nil
}

// Push publishes the current branch to its upstream.
func (client *Client) Push(ctx context.Context) error {
	_, err := client.run(ctx, "push")
	return err
}

// run executes a git command and returns its combined output.
func (client *Client) run(ctx context.Context, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = client.workingDirectory
	command.Env = append(os.Environ(),
		"LC_ALL=C",
		"GIT_TERMINAL_PROMPT=0",
	)

	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output

	if err := command.Run(); err != nil {
		return output.Bytes(), &CommandError{
			Arguments: arguments,
			Output:    output.String(),
			Err:       err,
		}
	}

	return output.Bytes(), nil
}

// CommandError describes a failed git invocation.
type CommandError struct {
	Arguments []string
	Output    string
	Err       error
}

func (commandError *CommandError) Error() string {
	return fmt.Sprintf("git %s: %v: %s",
		strings.Join(commandError.Arguments, " "),
		commandError.Err,
		strings.TrimSpace(commandError.Output),
	)
}

func (commandError *CommandError) Unwrap() error {
	return commandError.Err
}
