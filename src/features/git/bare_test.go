package git

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestBareRepositoriesCloneResolveAndArchive(t *testing.T) {
	initializeTestEnvironment(t)

	operationContext := context.Background()
	source := initializeTestRepository(t, operationContext)
	writeTestFile(t, source, "awake-plugin.yaml", "id: example\n")
	runTestGit(t, source, "add", "--all")
	runTestGit(t, source, "commit", "--message", "first")
	runTestGit(t, source, "tag", "v1.0.0")

	repositories := BareRepositories{}
	cache := filepath.Join(t.TempDir(), "example.git")
	if err := repositories.CloneBare(operationContext, "file://"+source, cache); err != nil {
		t.Fatalf("clone: %v", err)
	}

	tagged, err := repositories.ResolveCommit(operationContext, cache, "v1.0.0")
	if err != nil {
		t.Fatalf("resolve tag: %v", err)
	}
	head, err := repositories.ResolveCommit(operationContext, cache, "")
	if err != nil {
		t.Fatalf("resolve default branch: %v", err)
	}
	if tagged != head || len(tagged) != 40 {
		t.Fatalf("expected the tag and the default branch to name the same commit, got %q and %q", tagged, head)
	}

	var archive bytes.Buffer
	if err := repositories.Archive(operationContext, cache, tagged, &archive); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if content := readTarFile(t, &archive, "awake-plugin.yaml"); content != "id: example\n" {
		t.Errorf("unexpected archived content: %q", content)
	}
}

func TestBareRepositoriesFetchBringsNewCommitsAndTags(t *testing.T) {
	initializeTestEnvironment(t)

	operationContext := context.Background()
	source := initializeTestRepository(t, operationContext)
	writeTestFile(t, source, "notes.txt", "one\n")
	runTestGit(t, source, "add", "--all")
	runTestGit(t, source, "commit", "--message", "first")

	repositories := BareRepositories{}
	cache := filepath.Join(t.TempDir(), "notes.git")
	if err := repositories.CloneBare(operationContext, "file://"+source, cache); err != nil {
		t.Fatalf("clone: %v", err)
	}

	writeTestFile(t, source, "notes.txt", "two\n")
	runTestGit(t, source, "commit", "--all", "--message", "second")
	runTestGit(t, source, "tag", "v2.0.0")

	if _, err := repositories.ResolveCommit(operationContext, cache, "v2.0.0"); err == nil {
		t.Fatal("expected the new tag to be unknown before fetching")
	}
	if err := repositories.Fetch(operationContext, cache); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	commit, err := repositories.ResolveCommit(operationContext, cache, "v2.0.0")
	if err != nil {
		t.Fatalf("resolve the fetched tag: %v", err)
	}

	found, err := repositories.HasCommit(operationContext, cache, commit)
	if err != nil || !found {
		t.Errorf("expected the fetched commit to be present, got %v, %v", found, err)
	}
	missing, err := repositories.HasCommit(operationContext, cache, strings.Repeat("0", 40))
	if err != nil || missing {
		t.Errorf("expected an unknown commit to be missing without error, got %v, %v", missing, err)
	}
}

func TestBareRepositoriesRefuseOptionLikeRevisions(t *testing.T) {
	repositories := BareRepositories{}
	operationContext := context.Background()
	directory := t.TempDir()

	if _, err := repositories.ResolveCommit(operationContext, directory, "--output=/tmp/x"); err == nil {
		t.Error("expected ResolveCommit to refuse an option")
	}
	if _, err := repositories.HasCommit(operationContext, directory, "--all"); err == nil {
		t.Error("expected HasCommit to refuse an option")
	}
	if err := repositories.Archive(operationContext, directory, "--remote=x", io.Discard); err == nil {
		t.Error("expected Archive to refuse an option")
	}
}

func TestBareRepositoriesCloneReportsAnUnreachableRepository(t *testing.T) {
	initializeTestEnvironment(t)

	err := BareRepositories{}.CloneBare(context.Background(), "file://"+filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "clone.git"))

	var commandError *CommandError
	if !errors.As(err, &commandError) {
		t.Fatalf("expected a command error, got %v", err)
	}
}

func readTarFile(t *testing.T, archive io.Reader, name string) string {
	t.Helper()

	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			t.Fatalf("%s is not in the archive", name)
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		if header.Name == name {
			content, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return string(content)
		}
	}
}
