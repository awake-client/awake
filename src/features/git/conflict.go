package git

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// ConflictVersions holds the three versions of a conflicting path, as recorded
// by git in the index. A missing version is empty, for example the base of an
// add/add conflict.
type ConflictVersions struct {
	Base     []byte
	Local    []byte
	Incoming []byte
}

// ConflictVersions reads the base, local and incoming versions of a path that
// is currently in conflict.
func (client *Client) ConflictVersions(ctx context.Context, path string) (ConflictVersions, error) {
	base, err := client.stageContent(ctx, 1, path)
	if err != nil {
		return ConflictVersions{}, err
	}

	local, err := client.stageContent(ctx, 2, path)
	if err != nil {
		return ConflictVersions{}, err
	}

	incoming, err := client.stageContent(ctx, 3, path)
	if err != nil {
		return ConflictVersions{}, err
	}

	return ConflictVersions{
		Base:     base,
		Local:    local,
		Incoming: incoming,
	}, nil
}

func (client *Client) stageContent(ctx context.Context, stage int, path string) ([]byte, error) {
	output, err := client.run(ctx, "show", fmt.Sprintf(":%d:%s", stage, path))
	if err == nil {
		return output, nil
	}

	if isMissingStage(err) {
		return nil, nil
	}

	return nil, err
}

func isMissingStage(err error) bool {
	commandError, ok := err.(*CommandError)
	if !ok {
		return false
	}

	message := commandError.Output
	return strings.Contains(message, "exists on disk, but not in") ||
		strings.Contains(message, "does not exist in")
}

// Resolve marks a conflicted path as resolved. The caller is responsible for
// writing the resolved content to disk first.
func (client *Client) Resolve(ctx context.Context, path string) error {
	_, err := client.run(ctx, "add", "--", path)
	return err
}

// HasConflicts reports whether the index currently contains unmerged paths.
func (client *Client) HasConflicts(ctx context.Context) (bool, error) {
	output, err := client.run(ctx, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, err
	}

	return len(bytes.TrimSpace(output)) > 0, nil
}
