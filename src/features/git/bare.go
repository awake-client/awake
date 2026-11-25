package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/awake-client/awake/src/shared/process"
)

// headReference names the default branch of a cloned repository.
const headReference = "HEAD"

// disableExternalTransport stops git from running a command named by an
// ext:: URL, as the repositories cloned here come from third parties.
var disableExternalTransport = []string{"-c", "protocol.ext.allow=never"}

// BareRepositories clones and reads bare repositories that hold no work tree,
// such as the plugins and marketplaces Awake keeps in its cache. Every method
// takes the path of the bare repository.
type BareRepositories struct{}

// CloneBare clones url into directory without a work tree. The parent of
// directory must exist and directory itself must not.
func (BareRepositories) CloneBare(operationContext context.Context, url string, directory string) error {
	arguments := append(append([]string{}, disableExternalTransport...), "clone", "--bare", "--quiet", "--", url, directory)
	_, err := runBare(operationContext, "", arguments...)
	return err
}

// Fetch updates every branch and tag from the remote the repository was
// cloned from, removing the ones the remote deleted.
func (BareRepositories) Fetch(operationContext context.Context, directory string) error {
	arguments := append(append([]string{}, disableExternalTransport...),
		"fetch", "--quiet", "--prune", "--prune-tags", "--force", "origin",
		"+refs/heads/*:refs/heads/*",
		"+refs/tags/*:refs/tags/*",
	)
	_, err := runBare(operationContext, directory, arguments...)
	return err
}

// ResolveCommit returns the full hash of the commit a branch, tag or hash
// points at. An empty ref resolves the default branch.
func (BareRepositories) ResolveCommit(operationContext context.Context, directory string, ref string) (string, error) {
	if ref == "" {
		ref = headReference
	}
	if strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("git: %q is not a revision", ref)
	}

	output, err := runBare(operationContext, directory, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("git: %q names no commit: %w", ref, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// HasCommit reports whether the repository already holds commit, so a cached
// plugin needs no fetch.
func (BareRepositories) HasCommit(operationContext context.Context, directory string, commit string) (bool, error) {
	if strings.HasPrefix(commit, "-") {
		return false, fmt.Errorf("git: %q is not a revision", commit)
	}

	_, err := runBare(operationContext, directory, "cat-file", "-e", commit+"^{commit}")
	if err == nil {
		return true, nil
	}
	var commandError *CommandError
	var exitError *exec.ExitError
	if errors.As(err, &commandError) && errors.As(commandError.Err, &exitError) {
		return false, nil
	}
	return false, err
}

// Archive writes the files of commit to destination as a tar stream.
func (BareRepositories) Archive(operationContext context.Context, directory string, commit string, destination io.Writer) error {
	if strings.HasPrefix(commit, "-") {
		return fmt.Errorf("git: %q is not a revision", commit)
	}

	arguments := []string{"archive", "--format=tar", commit}
	command := bareCommand(operationContext, directory, arguments...)
	var errorOutput bytes.Buffer
	command.Stdout = destination
	command.Stderr = &errorOutput

	if err := command.Run(); err != nil {
		return &CommandError{Arguments: arguments, Output: errorOutput.String(), Err: err}
	}
	return nil
}

// runBare runs git against the bare repository at directory, or without a
// repository when directory is empty, and returns its combined output.
func runBare(operationContext context.Context, directory string, arguments ...string) ([]byte, error) {
	command := bareCommand(operationContext, directory, arguments...)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output

	if err := command.Run(); err != nil {
		return output.Bytes(), &CommandError{Arguments: arguments, Output: output.String(), Err: err}
	}
	return output.Bytes(), nil
}

// bareCommand names the repository with --git-dir instead of relying on the
// working directory, which git refuses for bare repositories when
// safe.bareRepository is set to explicit.
func bareCommand(operationContext context.Context, directory string, arguments ...string) *exec.Cmd {
	if directory != "" {
		arguments = append([]string{"--git-dir=" + directory}, arguments...)
	}
	command := exec.CommandContext(operationContext, "git", arguments...)
	command.Env = append(os.Environ(),
		"LC_ALL=C",
		"GIT_TERMINAL_PROMPT=0",
	)
	process.HideConsoleWindow(command)
	return command
}
