package git

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

// Status describes the state of the work tree.
type Status struct {
	Branch   string
	Upstream string
	Ahead    int
	Behind   int
	Changes  []Change
}

// Change describes a single modified path.
type Change struct {
	Path           string
	OriginalPath   string
	IndexStatus    string
	WorktreeStatus string
	Untracked      bool
	Conflicted     bool
}

// ConflictedPaths returns the paths that require a manual merge.
func (status Status) ConflictedPaths() []string {
	paths := make([]string, 0, len(status.Changes))
	for _, change := range status.Changes {
		if change.Conflicted {
			paths = append(paths, change.Path)
		}
	}
	return paths
}

// PullResult describes the outcome of a pull.
type PullResult struct {
	AlreadyUpToDate bool
	Conflicts       []string
}

// Status reads the current branch and the work tree changes.
func (client *Client) Status(ctx context.Context) (Status, error) {
	output, err := client.run(ctx, "status", "--porcelain=v1", "-z", "--branch")
	if err != nil {
		return Status{}, err
	}

	return parseStatus(output), nil
}

func parseStatus(output []byte) Status {
	status := Status{}
	fields := bytes.Split(output, []byte{0})

	for index := 0; index < len(fields); index++ {
		field := string(fields[index])
		if field == "" {
			continue
		}

		if strings.HasPrefix(field, "## ") {
			applyBranchLine(&status, strings.TrimPrefix(field, "## "))
			continue
		}

		if len(field) < 4 {
			continue
		}

		change := Change{
			IndexStatus:    field[0:1],
			WorktreeStatus: field[1:2],
			Path:           field[3:],
		}
		change.Untracked = change.IndexStatus == "?" && change.WorktreeStatus == "?"
		change.Conflicted = isConflicted(change.IndexStatus, change.WorktreeStatus)

		if (change.IndexStatus == "R" || change.IndexStatus == "C") && index+1 < len(fields) {
			index++
			change.OriginalPath = string(fields[index])
		}

		status.Changes = append(status.Changes, change)
	}

	return status
}

func applyBranchLine(status *Status, line string) {
	tracking := ""
	if position := strings.Index(line, " ["); position >= 0 {
		tracking = strings.TrimSuffix(line[position+2:], "]")
		line = line[:position]
	}

	switch {
	case strings.HasPrefix(line, "No commits yet on "):
		status.Branch = strings.TrimPrefix(line, "No commits yet on ")
	case strings.Contains(line, "..."):
		separator := strings.Index(line, "...")
		status.Branch = line[:separator]
		status.Upstream = line[separator+3:]
	default:
		status.Branch = line
	}

	for _, entry := range strings.Split(tracking, ", ") {
		switch {
		case strings.HasPrefix(entry, "ahead "):
			status.Ahead, _ = strconv.Atoi(strings.TrimPrefix(entry, "ahead "))
		case strings.HasPrefix(entry, "behind "):
			status.Behind, _ = strconv.Atoi(strings.TrimPrefix(entry, "behind "))
		}
	}
}

func isConflicted(indexStatus string, worktreeStatus string) bool {
	conflictedPairs := map[string]bool{
		"DD": true,
		"AU": true,
		"UD": true,
		"UA": true,
		"DU": true,
		"AA": true,
		"UU": true,
	}

	return conflictedPairs[indexStatus+worktreeStatus]
}
