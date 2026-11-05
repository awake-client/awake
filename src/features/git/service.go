package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const operationTimeout = 60 * time.Second

// Service exposes git operations to the application layer.
type Service struct{}

// NewService returns a git service.
func NewService() *Service {
	return &Service{}
}

// IsRepository reports whether the workspace is tracked by git.
func (service *Service) IsRepository(workspaceRoot string) (bool, error) {
	requestContext, cancel := contextForOperation()
	defer cancel()

	return (&Client{workingDirectory: workspaceRoot}).IsRepository(requestContext)
}

// Initialize creates a repository in the workspace.
func (service *Service) Initialize(workspaceRoot string, initialBranch string) error {
	requestContext, cancel := contextForOperation()
	defer cancel()

	if strings.TrimSpace(initialBranch) == "" {
		initialBranch = "main"
	}

	return Init(requestContext, workspaceRoot, initialBranch)
}

// Status returns the branch and work tree changes.
func (service *Service) Status(workspaceRoot string) (Status, error) {
	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return Status{}, err
	}

	return client.Status(requestContext)
}

// Stage adds paths to the index.
func (service *Service) Stage(workspaceRoot string, paths []string) error {
	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return err
	}

	return client.Stage(requestContext, paths...)
}

// Commit stages every change and creates a commit.
func (service *Service) Commit(workspaceRoot string, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("git: commit message is required")
	}

	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return "", err
	}

	if err := client.StageAll(requestContext); err != nil {
		return "", err
	}

	return client.Commit(requestContext, message)
}

// Pull fetches and merges the upstream branch.
func (service *Service) Pull(workspaceRoot string) (PullResult, error) {
	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return PullResult{}, err
	}

	return client.Pull(requestContext)
}

// Push publishes the current branch.
func (service *Service) Push(workspaceRoot string) error {
	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return err
	}

	return client.Push(requestContext)
}

// HasConflicts reports whether the index contains unmerged paths.
func (service *Service) HasConflicts(workspaceRoot string) (bool, error) {
	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return false, err
	}

	return client.HasConflicts(requestContext)
}

// ConflictVersions returns the base, local and incoming content of a path.
func (service *Service) ConflictVersions(workspaceRoot string, path string) (ConflictVersions, error) {
	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return ConflictVersions{}, err
	}

	return client.ConflictVersions(requestContext, path)
}

// ResolveWith writes content to a conflicted path and marks it resolved.
func (service *Service) ResolveWith(workspaceRoot string, path string, content string) error {
	resolvedPath, err := resolveWithinRoot(workspaceRoot, path)
	if err != nil {
		return err
	}

	requestContext, cancel := contextForOperation()
	defer cancel()

	client, err := Open(requestContext, workspaceRoot)
	if err != nil {
		return err
	}

	if err := os.WriteFile(resolvedPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write resolved path: %w", err)
	}

	return client.Resolve(requestContext, path)
}

func contextForOperation() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), operationTimeout)
}

func resolveWithinRoot(workspaceRoot string, path string) (string, error) {
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	resolved := filepath.Join(root, filepath.FromSlash(path))
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("git: path escapes the workspace")
	}

	return resolved, nil
}
