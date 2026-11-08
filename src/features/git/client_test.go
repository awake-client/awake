package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenRejectsDirectoryOutsideRepository(t *testing.T) {
	initializeTestEnvironment(t)

	_, err := Open(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected an error for a directory outside a repository")
	}
}

func TestStatusReportsModifiedAndUntrackedFiles(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	repository := initializeTestRepository(t, ctx)

	writeTestFile(t, repository, "workspace.yaml", "name: one\n")

	client := openTestClient(t, ctx, repository)
	if err := client.Stage(ctx, "workspace.yaml"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := client.Commit(ctx, "initial commit"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	writeTestFile(t, repository, "workspace.yaml", "name: two\n")
	writeTestFile(t, repository, "notes.yaml", "remember\n")

	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	if status.Branch != "main" {
		t.Errorf("expected branch main, got %q", status.Branch)
	}
	if len(status.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(status.Changes), status.Changes)
	}

	modified := findChange(t, status, "workspace.yaml")
	if modified.WorktreeStatus != "M" {
		t.Errorf("expected workspace.yaml to be modified, got %+v", modified)
	}

	untracked := findChange(t, status, "notes.yaml")
	if !untracked.Untracked {
		t.Errorf("expected notes.yaml to be untracked, got %+v", untracked)
	}
}

func TestPullReportsConflictVersions(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", remote)

	localDirectory := filepath.Join(root, "local")
	incomingDirectory := filepath.Join(root, "incoming")
	runTestGit(t, root, "clone", remote, localDirectory)
	runTestGit(t, root, "clone", remote, incomingDirectory)

	writeTestFile(t, localDirectory, "workspace.yaml", "name: base\n")

	localClient := openTestClient(t, ctx, localDirectory)
	if err := localClient.Stage(ctx, "workspace.yaml"); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if _, err := localClient.Commit(ctx, "add workspace"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	runTestGit(t, localDirectory, "push", "--set-upstream", "origin", "main")

	incomingClient := openTestClient(t, ctx, incomingDirectory)
	if _, err := incomingClient.Pull(ctx); err != nil {
		t.Fatalf("incoming pull: %v", err)
	}
	writeTestFile(t, incomingDirectory, "workspace.yaml", "name: incoming\n")
	if err := incomingClient.Stage(ctx, "workspace.yaml"); err != nil {
		t.Fatalf("incoming stage: %v", err)
	}
	if _, err := incomingClient.Commit(ctx, "rename workspace"); err != nil {
		t.Fatalf("incoming commit: %v", err)
	}
	if err := incomingClient.Push(ctx); err != nil {
		t.Fatalf("incoming push: %v", err)
	}

	writeTestFile(t, localDirectory, "workspace.yaml", "name: local\n")
	if err := localClient.Stage(ctx, "workspace.yaml"); err != nil {
		t.Fatalf("local stage: %v", err)
	}
	if _, err := localClient.Commit(ctx, "rename workspace locally"); err != nil {
		t.Fatalf("local commit: %v", err)
	}

	result, err := localClient.Pull(ctx)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0] != "workspace.yaml" {
		t.Fatalf("expected one conflict on workspace.yaml, got %+v", result)
	}

	versions, err := localClient.ConflictVersions(ctx, "workspace.yaml")
	if err != nil {
		t.Fatalf("conflict versions: %v", err)
	}
	assertFileContent(t, "base", versions.Base, "name: base\n")
	assertFileContent(t, "local", versions.Local, "name: local\n")
	assertFileContent(t, "incoming", versions.Incoming, "name: incoming\n")

	writeTestFile(t, localDirectory, "workspace.yaml", "name: resolved\n")
	if err := localClient.Resolve(ctx, "workspace.yaml"); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	hasConflicts, err := localClient.HasConflicts(ctx)
	if err != nil {
		t.Fatalf("has conflicts: %v", err)
	}
	if hasConflicts {
		t.Fatal("expected conflicts to be resolved")
	}
}

func TestSetRemoteURLAddsAndUpdatesOrigin(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	repository := initializeTestRepository(t, ctx)
	client := openTestClient(t, ctx, repository)

	url, err := client.RemoteURL(ctx, "origin")
	if err != nil {
		t.Fatalf("read missing remote: %v", err)
	}
	if url != "" {
		t.Fatalf("expected no remote, got %q", url)
	}

	if err := client.SetRemoteURL(ctx, "origin", "https://example.com/team/workspace.git"); err != nil {
		t.Fatalf("add remote: %v", err)
	}
	url, err = client.RemoteURL(ctx, "origin")
	if err != nil {
		t.Fatalf("read added remote: %v", err)
	}
	if url != "https://example.com/team/workspace.git" {
		t.Errorf("expected the added URL, got %q", url)
	}

	if err := client.SetRemoteURL(ctx, "origin", "git@example.com:team/other.git"); err != nil {
		t.Fatalf("update remote: %v", err)
	}
	url, err = client.RemoteURL(ctx, "origin")
	if err != nil {
		t.Fatalf("read updated remote: %v", err)
	}
	if url != "git@example.com:team/other.git" {
		t.Errorf("expected the updated URL, got %q", url)
	}
}

func TestPushSetsUpstreamOnFirstPush(t *testing.T) {
	initializeTestEnvironment(t)

	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", remote)

	repository := filepath.Join(root, "workspace")
	runTestGit(t, root, "init", "--initial-branch=main", repository)
	writeTestFile(t, repository, "awake.yaml", "name: team\n")
	runTestGit(t, repository, "add", "awake.yaml")
	runTestGit(t, repository, "commit", "-m", "initial commit")

	client := openTestClient(t, ctx, repository)
	if err := client.SetRemoteURL(ctx, "origin", remote); err != nil {
		t.Fatalf("add remote: %v", err)
	}

	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("status before push: %v", err)
	}
	if status.Upstream != "" {
		t.Fatalf("expected no upstream before push, got %q", status.Upstream)
	}

	if err := client.Push(ctx); err != nil {
		t.Fatalf("push: %v", err)
	}

	status, err = client.Status(ctx)
	if err != nil {
		t.Fatalf("status after push: %v", err)
	}
	if status.Upstream != "origin/main" {
		t.Errorf("expected the push to set origin/main as upstream, got %q", status.Upstream)
	}
}

func TestParseStatusReadsTrackingInformation(t *testing.T) {
	status := parseStatus([]byte("## main...origin/main [ahead 2, behind 3]\x00"))

	if status.Branch != "main" {
		t.Errorf("expected branch main, got %q", status.Branch)
	}
	if status.Upstream != "origin/main" {
		t.Errorf("expected upstream origin/main, got %q", status.Upstream)
	}
	if status.Ahead != 2 || status.Behind != 3 {
		t.Errorf("expected ahead 2 behind 3, got ahead %d behind %d", status.Ahead, status.Behind)
	}
}

func TestParseStatusReadsBranchWithoutCommits(t *testing.T) {
	status := parseStatus([]byte("## No commits yet on main\x00"))

	if status.Branch != "main" {
		t.Errorf("expected branch main, got %q", status.Branch)
	}
}

func initializeTestEnvironment(t *testing.T) {
	t.Helper()

	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_AUTHOR_NAME", "Awake Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@awake.local")
	t.Setenv("GIT_COMMITTER_NAME", "Awake Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@awake.local")
}

func initializeTestRepository(t *testing.T, ctx context.Context) string {
	t.Helper()

	directory := t.TempDir()
	if err := Init(ctx, directory, "main"); err != nil {
		t.Fatalf("init repository: %v", err)
	}
	return directory
}

func openTestClient(t *testing.T, ctx context.Context, directory string) *Client {
	t.Helper()

	client, err := Open(ctx, directory)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	return client
}

func runTestGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()

	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}

func writeTestFile(t *testing.T, directory string, path string, content string) {
	t.Helper()

	fullPath := filepath.Join(directory, path)
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", fullPath, err)
	}
}

func findChange(t *testing.T, status Status, path string) Change {
	t.Helper()

	for _, change := range status.Changes {
		if change.Path == path {
			return change
		}
	}

	t.Fatalf("no change found for %s", path)
	return Change{}
}

func assertFileContent(t *testing.T, name string, content []byte, expected string) {
	t.Helper()

	if string(content) != expected {
		t.Errorf("unexpected %s content: expected %q, got %q", name, expected, content)
	}
}
