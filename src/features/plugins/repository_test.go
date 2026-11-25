package plugins

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testRepositoryURL = "https://example.com/awake-plugins/example.git"
	firstCommit       = "1111111111111111111111111111111111111111"
	secondCommit      = "2222222222222222222222222222222222222222"
)

func TestCacheResolveClonesThenFetches(t *testing.T) {
	git := newFakeGit()
	git.publish(firstCommit, "v1.0.0", testArchive(t, map[string]string{"awake-plugin.yaml": "id: example\n"}))
	cache := NewCache(t.TempDir(), git)
	operationContext := context.Background()

	commit, err := cache.Resolve(operationContext, testRepositoryURL, "v1.0.0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if commit != firstCommit || git.clones != 1 || git.fetches != 0 {
		t.Fatalf("expected one clone and no fetch, got commit %s, %d clones, %d fetches", commit, git.clones, git.fetches)
	}

	git.publish(secondCommit, "v2.0.0", testArchive(t, map[string]string{"awake-plugin.yaml": "id: example\n"}))
	commit, err = cache.Resolve(operationContext, testRepositoryURL, "v2.0.0")
	if err != nil {
		t.Fatalf("resolve after publishing: %v", err)
	}
	if commit != secondCommit || git.clones != 1 || git.fetches != 1 {
		t.Errorf("expected the second resolve to fetch, got commit %s, %d clones, %d fetches", commit, git.clones, git.fetches)
	}
}

func TestCacheCheckoutExtractsOnceAndWorksOffline(t *testing.T) {
	git := newFakeGit()
	git.publish(firstCommit, "", testArchive(t, map[string]string{
		"awake-plugin.yaml": string(readTestData(t, "complete-manifest.yaml")),
		"dist/plugin.js":    "awake.register({})\n",
	}))
	cache := NewCache(t.TempDir(), git)
	operationContext := context.Background()

	if _, found := cache.CachedCheckout(testRepositoryURL, firstCommit); found {
		t.Fatal("expected nothing cached before the first checkout")
	}
	directory, err := cache.Checkout(operationContext, testRepositoryURL, firstCommit)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	manifest, err := ReadManifest(directory)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	script, err := ReadFile(directory, manifest.Main)
	if err != nil || string(script) != "awake.register({})\n" {
		t.Fatalf("unexpected main script %q: %v", script, err)
	}

	git.offline = true
	again, err := cache.Checkout(operationContext, testRepositoryURL, firstCommit)
	if err != nil || again != directory {
		t.Fatalf("expected the cached checkout without git, got %q: %v", again, err)
	}
	cached, found := cache.CachedCheckout(testRepositoryURL, firstCommit)
	if !found || cached != directory {
		t.Errorf("expected CachedCheckout to find %q, got %q", directory, cached)
	}
	if git.archives != 1 {
		t.Errorf("expected one archive, got %d", git.archives)
	}
}

func TestCacheCheckoutFetchesAMissingCommit(t *testing.T) {
	git := newFakeGit()
	git.publish(firstCommit, "", testArchive(t, map[string]string{"one.txt": "1"}))
	cache := NewCache(t.TempDir(), git)
	operationContext := context.Background()
	if _, err := cache.Checkout(operationContext, testRepositoryURL, firstCommit); err != nil {
		t.Fatalf("first checkout: %v", err)
	}

	git.publish(secondCommit, "", testArchive(t, map[string]string{"two.txt": "2"}))
	directory, err := cache.Checkout(operationContext, testRepositoryURL, secondCommit)
	if err != nil {
		t.Fatalf("second checkout: %v", err)
	}
	if git.fetches != 1 {
		t.Errorf("expected one fetch for the missing commit, got %d", git.fetches)
	}
	if _, err := os.Stat(filepath.Join(directory, "two.txt")); err != nil {
		t.Errorf("expected two.txt in the second checkout: %v", err)
	}
}

func TestCacheCheckoutReportsAnUnknownCommit(t *testing.T) {
	git := newFakeGit()
	git.publish(firstCommit, "", testArchive(t, map[string]string{"one.txt": "1"}))
	cache := NewCache(t.TempDir(), git)

	_, err := cache.Checkout(context.Background(), testRepositoryURL, secondCommit)

	if err == nil || !strings.Contains(err.Error(), "has no commit") {
		t.Fatalf("expected a missing commit error, got %v", err)
	}
}

func TestCacheCheckoutRefusesInvalidInput(t *testing.T) {
	cache := NewCache(t.TempDir(), newFakeGit())
	operationContext := context.Background()

	if _, err := cache.Checkout(operationContext, testRepositoryURL, "v1.0.0"); !errors.Is(err, ErrInvalidCommit) {
		t.Errorf("expected ErrInvalidCommit for a tag, got %v", err)
	}
	if _, err := cache.Checkout(operationContext, "http://example.com/x.git", firstCommit); !errors.Is(err, ErrUnsupportedRepositoryURL) {
		t.Errorf("expected ErrUnsupportedRepositoryURL, got %v", err)
	}
	if _, err := cache.Resolve(operationContext, testRepositoryURL, "--upload-pack=x"); err == nil {
		t.Error("expected an option-like ref to be refused")
	}
}

func TestCacheCheckoutRefusesUnsafeArchives(t *testing.T) {
	cases := []struct {
		name     string
		entries  []tar.Header
		expected error
	}{
		{"parent directory", []tar.Header{fileHeader("../escape.txt", 1)}, ErrUnsafePath},
		{"absolute path", []tar.Header{fileHeader("/etc/escape.txt", 1)}, ErrUnsafePath},
		{"too large", []tar.Header{fileHeader("big.bin", maximumCheckoutSize+1)}, ErrCheckoutTooLarge},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			git := newFakeGit()
			git.publish(firstCommit, "", rawArchive(t, test.entries))
			root := t.TempDir()
			cache := NewCache(root, git)

			_, err := cache.Checkout(context.Background(), testRepositoryURL, firstCommit)

			if !errors.Is(err, test.expected) {
				t.Fatalf("expected %v, got %v", test.expected, err)
			}
			if _, found := cache.CachedCheckout(testRepositoryURL, firstCommit); found {
				t.Error("a refused archive must not leave a checkout behind")
			}
			assertNoStagingLeft(t, root)
		})
	}
}

func TestCacheCheckoutSkipsSymbolicLinks(t *testing.T) {
	git := newFakeGit()
	git.publish(firstCommit, "", rawArchive(t, []tar.Header{
		{Name: "secrets", Typeflag: tar.TypeSymlink, Linkname: "/etc", Mode: 0o777},
		fileHeader("kept.txt", 1),
	}))
	cache := NewCache(t.TempDir(), git)

	directory, err := cache.Checkout(context.Background(), testRepositoryURL, firstCommit)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(directory, "secrets")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected the symbolic link to be skipped, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "kept.txt")); err != nil {
		t.Errorf("expected kept.txt: %v", err)
	}
}

func TestCacheCheckoutLeavesNothingWhenTheArchiveFails(t *testing.T) {
	git := newFakeGit()
	git.publish(firstCommit, "", testArchive(t, map[string]string{"one.txt": "1"}))
	git.archiveError = errors.New("disk full")
	root := t.TempDir()
	cache := NewCache(root, git)

	_, err := cache.Checkout(context.Background(), testRepositoryURL, firstCommit)

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected the archive error, got %v", err)
	}
	if _, found := cache.CachedCheckout(testRepositoryURL, firstCommit); found {
		t.Error("a failed archive must not leave a checkout behind")
	}
	assertNoStagingLeft(t, root)
}

func TestReadFileRefusesPathsLeavingTheCheckout(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	checkout := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(checkout, "link.txt")); err != nil {
		t.Skipf("symbolic links are not available: %v", err)
	}
	if err := os.Mkdir(filepath.Join(checkout, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, relative := range []string{"link.txt", "../secret.txt", "/etc/hosts"} {
		if _, err := ReadFile(checkout, relative); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("expected ErrUnsafePath for %q, got %v", relative, err)
		}
	}
	if _, err := ReadFile(checkout, "folder"); err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Errorf("expected a directory to be refused, got %v", err)
	}
}

// fakeGit serves a single repository whose commits are tar archives held in
// memory. Commits become visible to the cache when it clones or fetches.
type fakeGit struct {
	remote       map[string][]byte
	remoteRefs   map[string]string
	local        map[string]map[string][]byte
	localRefs    map[string]map[string]string
	offline      bool
	archiveError error
	clones       int
	fetches      int
	archives     int
}

func newFakeGit() *fakeGit {
	return &fakeGit{
		remote:     map[string][]byte{},
		remoteRefs: map[string]string{},
		local:      map[string]map[string][]byte{},
		localRefs:  map[string]map[string]string{},
	}
}

func (git *fakeGit) publish(commit string, tag string, archive []byte) {
	git.remote[commit] = archive
	git.remoteRefs[headReferenceForTests] = commit
	if tag != "" {
		git.remoteRefs[tag] = commit
	}
}

const headReferenceForTests = ""

func (git *fakeGit) CloneBare(_ context.Context, _ string, directory string) error {
	if git.offline {
		return errors.New("offline")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	git.clones++
	git.sync(filepath.Base(directory))
	return nil
}

func (git *fakeGit) Fetch(_ context.Context, directory string) error {
	if git.offline {
		return errors.New("offline")
	}
	git.fetches++
	git.sync(filepath.Base(directory))
	return nil
}

// sync copies the remote state under the base name of the repository, which
// survives the rename from the staging directory.
func (git *fakeGit) sync(name string) {
	git.local[name] = maps.Clone(git.remote)
	git.localRefs[name] = maps.Clone(git.remoteRefs)
}

func (git *fakeGit) ResolveCommit(_ context.Context, directory string, ref string) (string, error) {
	commit, found := git.localRefs[filepath.Base(directory)][ref]
	if !found {
		return "", fmt.Errorf("unknown ref %q", ref)
	}
	return commit, nil
}

func (git *fakeGit) HasCommit(_ context.Context, directory string, commit string) (bool, error) {
	_, found := git.local[filepath.Base(directory)][commit]
	return found, nil
}

func (git *fakeGit) Archive(_ context.Context, directory string, commit string, destination io.Writer) error {
	git.archives++
	if git.archiveError != nil {
		return git.archiveError
	}
	archive, found := git.local[filepath.Base(directory)][commit]
	if !found {
		return fmt.Errorf("unknown commit %s", commit)
	}
	_, err := destination.Write(archive)
	return err
}

func testArchive(t *testing.T, contents map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for name, content := range contents {
		header := fileHeader(name, int64(len(content)))
		if err := writer.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// rawArchive writes headers with zero filled contents, which lets a test
// declare a huge file without allocating it: extraction stops at the header.
func rawArchive(t *testing.T, headers []tar.Header) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, header := range headers {
		if err := writer.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg && header.Size <= maximumCheckoutSize {
			if _, err := writer.Write(make([]byte, header.Size)); err != nil {
				t.Fatal(err)
			}
		}
	}
	// An oversized entry leaves the archive unfinished on purpose.
	_ = writer.Flush()
	return buffer.Bytes()
}

func fileHeader(name string, size int64) tar.Header {
	return tar.Header{Name: name, Typeflag: tar.TypeReg, Size: size, Mode: 0o644}
}

func assertNoStagingLeft(t *testing.T, root string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(entry.Name(), partialCheckoutSuffix) {
			t.Errorf("staging directory left behind: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
