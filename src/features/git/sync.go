package git

import "context"

// SyncOutcome tells what an automatic synchronization did.
type SyncOutcome string

const (
	// SyncSkipped means the workspace has no repository or no upstream branch.
	SyncSkipped SyncOutcome = "skipped"
	// SyncDone means the requested pull and push ran without conflict.
	SyncDone SyncOutcome = "done"
	// SyncConflicts means conflicts must be resolved before syncing again.
	SyncConflicts SyncOutcome = "conflicts"
)

// automaticCommitMessage is the message of the commits created automatically
// when the workspace enables automatic commits.
const automaticCommitMessage = "Auto-commit from Awake"

// SyncResult describes one automatic synchronization.
type SyncResult struct {
	Outcome SyncOutcome
	// Reason explains a skipped synchronization.
	Reason string
	// Committed is true when pending changes were committed automatically.
	Committed bool
	// Pulled is true when the pull brought new commits.
	Pulled    bool
	Pushed    bool
	Conflicts []string
	Ahead     int
	Behind    int
}

// Synchronize commits, pulls then pushes the current branch, as configured.
// It commits pending changes first so a pull never runs over a dirty work
// tree, never pulls over unresolved conflicts, and only pushes when local
// commits are ahead of the upstream branch, so it is safe to call on a timer.
func (service *Service) Synchronize(workspaceRoot string, commit bool, pull bool, push bool) (SyncResult, error) {
	requestContext, cancel := contextForOperation()
	defer cancel()

	client := &Client{workingDirectory: workspaceRoot}
	isRepository, err := client.IsRepository(requestContext)
	if err != nil {
		return SyncResult{}, err
	}
	if !isRepository {
		return SyncResult{Outcome: SyncSkipped, Reason: "The workspace is not a git repository."}, nil
	}

	return synchronize(requestContext, client, commit, pull, push)
}

func synchronize(ctx context.Context, client *Client, commit bool, pull bool, push bool) (SyncResult, error) {
	status, err := client.Status(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	if conflicts := status.ConflictedPaths(); len(conflicts) > 0 {
		return SyncResult{Outcome: SyncConflicts, Conflicts: conflicts, Ahead: status.Ahead, Behind: status.Behind}, nil
	}

	result := SyncResult{Outcome: SyncDone}
	if commit && len(status.Changes) > 0 {
		if err := client.StageAll(ctx); err != nil {
			return SyncResult{}, err
		}
		if _, err := client.Commit(ctx, automaticCommitMessage); err != nil {
			return SyncResult{}, err
		}
		result.Committed = true
		status, err = client.Status(ctx)
		if err != nil {
			return SyncResult{}, err
		}
	}

	if status.Upstream == "" {
		if result.Committed {
			result.Reason = "Committed locally; the current branch has no upstream branch."
			return result, nil
		}
		return SyncResult{Outcome: SyncSkipped, Reason: "The current branch has no upstream branch."}, nil
	}

	if pull {
		pullResult, err := client.Pull(ctx)
		if err != nil {
			return SyncResult{}, err
		}
		if len(pullResult.Conflicts) > 0 {
			result.Outcome = SyncConflicts
			result.Conflicts = pullResult.Conflicts
			return result, nil
		}
		result.Pulled = !pullResult.AlreadyUpToDate
	}

	status, err = client.Status(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	if push && status.Ahead > 0 {
		if err := client.Push(ctx); err != nil {
			return SyncResult{}, err
		}
		result.Pushed = true
		status, err = client.Status(ctx)
		if err != nil {
			return SyncResult{}, err
		}
	}

	result.Ahead = status.Ahead
	result.Behind = status.Behind
	return result, nil
}
