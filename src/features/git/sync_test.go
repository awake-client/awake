package git

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSynchronizePullsAndPushes(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	runTestGit(t, root, "clone", remote, first)

	writeTestFile(t, first, "awake.yaml", "name: base\n")
	runTestGit(t, first, "add", "awake.yaml")
	runTestGit(t, first, "commit", "-m", "base")
	runTestGit(t, first, "push", "--set-upstream", "origin", "main")
	runTestGit(t, root, "clone", remote, second)

	// A commit made in the first clone is pushed by its synchronization...
	writeTestFile(t, first, "request.yaml", "name: ping\n")
	runTestGit(t, first, "add", "request.yaml")
	runTestGit(t, first, "commit", "-m", "add request")

	pushed, err := synchronize(ctx, openTestClient(t, ctx, first), false, true, true)
	if err != nil {
		t.Fatalf("synchronize first: %v", err)
	}
	if pushed.Outcome != SyncDone || !pushed.Pushed || pushed.Ahead != 0 {
		t.Fatalf("expected the local commit to be pushed, got %+v", pushed)
	}

	// ...and pulled by the synchronization of the second clone.
	pulled, err := synchronize(ctx, openTestClient(t, ctx, second), false, true, true)
	if err != nil {
		t.Fatalf("synchronize second: %v", err)
	}
	if pulled.Outcome != SyncDone || !pulled.Pulled || pulled.Pushed {
		t.Fatalf("expected the second clone to pull without pushing, got %+v", pulled)
	}

	again, err := synchronize(ctx, openTestClient(t, ctx, second), false, true, true)
	if err != nil {
		t.Fatalf("synchronize again: %v", err)
	}
	if again.Pulled || again.Pushed {
		t.Fatalf("a second synchronization must be a no-op, got %+v", again)
	}
}

func TestSynchronizeStopsOnConflicts(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	runTestGit(t, root, "clone", remote, first)
	writeTestFile(t, first, "awake.yaml", "name: base\n")
	runTestGit(t, first, "add", "awake.yaml")
	runTestGit(t, first, "commit", "-m", "base")
	runTestGit(t, first, "push", "--set-upstream", "origin", "main")
	runTestGit(t, root, "clone", remote, second)

	writeTestFile(t, first, "awake.yaml", "name: first\n")
	runTestGit(t, first, "commit", "-am", "first")
	runTestGit(t, first, "push")
	writeTestFile(t, second, "awake.yaml", "name: second\n")
	runTestGit(t, second, "commit", "-am", "second")

	result, err := synchronize(ctx, openTestClient(t, ctx, second), false, true, true)
	if err != nil {
		t.Fatalf("synchronize: %v", err)
	}
	if result.Outcome != SyncConflicts || len(result.Conflicts) != 1 || result.Pushed {
		t.Fatalf("expected a conflict and no push, got %+v", result)
	}

	again, err := synchronize(ctx, openTestClient(t, ctx, second), false, true, true)
	if err != nil {
		t.Fatalf("synchronize again: %v", err)
	}
	if again.Outcome != SyncConflicts {
		t.Fatalf("expected the next synchronization to stop on the open conflict, got %+v", again)
	}
}

func TestSynchronizeCommitsPendingChanges(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	repository := filepath.Join(root, "workspace")
	runTestGit(t, root, "clone", remote, repository)
	writeTestFile(t, repository, "awake.yaml", "name: base\n")
	runTestGit(t, repository, "add", "awake.yaml")
	runTestGit(t, repository, "commit", "-m", "base")
	runTestGit(t, repository, "push", "--set-upstream", "origin", "main")

	writeTestFile(t, repository, "request.yaml", "name: ping\n")
	client := openTestClient(t, ctx, repository)

	result, err := synchronize(ctx, client, true, true, true)
	if err != nil {
		t.Fatalf("synchronize: %v", err)
	}
	if !result.Committed || !result.Pushed || result.Ahead != 0 {
		t.Fatalf("expected the pending change to be committed and pushed, got %+v", result)
	}

	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status.Changes) != 0 {
		t.Fatalf("expected a clean work tree, got %+v", status.Changes)
	}
}

func TestSynchronizeCommitsWithoutUpstream(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	repository := initializeTestRepository(t, ctx)
	writeTestFile(t, repository, "awake.yaml", "name: solo\n")

	result, err := synchronize(ctx, openTestClient(t, ctx, repository), true, true, true)
	if err != nil {
		t.Fatalf("synchronize: %v", err)
	}
	if result.Outcome != SyncDone || !result.Committed || result.Pushed {
		t.Fatalf("expected a local commit without a push, got %+v", result)
	}
	if result.Reason == "" {
		t.Fatal("expected a reason explaining that the branch has no upstream")
	}
}

func TestSynchronizeSkipsBranchesWithoutUpstream(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	repository := initializeTestRepository(t, ctx)
	writeTestFile(t, repository, "awake.yaml", "name: solo\n")
	runTestGit(t, repository, "add", "awake.yaml")
	runTestGit(t, repository, "commit", "-m", "solo")

	result, err := synchronize(ctx, openTestClient(t, ctx, repository), false, true, true)
	if err != nil {
		t.Fatalf("synchronize: %v", err)
	}
	if result.Outcome != SyncSkipped {
		t.Fatalf("expected a skipped synchronization, got %+v", result)
	}
}
