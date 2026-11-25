package plugins

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	repositoriesDirectoryName = "repositories"
	checkoutsDirectoryName    = "checkouts"
	partialCheckoutSuffix     = ".partial-"
	bareRepositorySuffix      = ".git"
	repositoryKeyLength       = 16

	// maximumCheckoutSize bounds the files extracted from one plugin commit,
	// so a hostile repository cannot fill the disk.
	maximumCheckoutSize = 64 * 1024 * 1024
	// maximumCheckoutFiles bounds the number of files of one plugin commit.
	maximumCheckoutFiles = 10_000

	checkoutDirectoryMode = 0o755
	checkoutFileMode      = 0o644
)

var (
	// ErrInvalidCommit is returned for a commit that is not a full hash.
	ErrInvalidCommit = errors.New("plugins: commit must be a full hexadecimal hash")
	// ErrCheckoutTooLarge is returned when a plugin commit holds more files or
	// bytes than Awake extracts.
	ErrCheckoutTooLarge = errors.New("plugins: plugin repository is too large")
	// ErrUnsafePath is returned for a path that leaves the plugin repository.
	ErrUnsafePath = errors.New("plugins: path leaves the plugin repository")

	errExtractionStopped = errors.New("plugins: extraction stopped")
)

// fullCommitHash matches SHA-1 and SHA-256 object names.
var fullCommitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// GitRepositories is what the cache needs from git. The git feature provides
// it with git.BareRepositories.
type GitRepositories interface {
	CloneBare(operationContext context.Context, url string, directory string) error
	Fetch(operationContext context.Context, directory string) error
	ResolveCommit(operationContext context.Context, directory string, ref string) (string, error)
	HasCommit(operationContext context.Context, directory string, commit string) (bool, error)
	Archive(operationContext context.Context, directory string, commit string, destination io.Writer) error
}

// Cache keeps a bare clone of every plugin and marketplace repository Awake
// uses, and the files of each commit it runs, extracted once and never
// changed. Everything in it can be downloaded again, so it lives in the user
// cache directory.
type Cache struct {
	root  string
	git   GitRepositories
	mutex sync.Mutex
}

// NewCache returns a cache stored under root.
func NewCache(root string, git GitRepositories) *Cache {
	return &Cache{root: root, git: git}
}

// DefaultCacheRoot returns the directory the cache uses outside tests.
func DefaultCacheRoot() (string, error) {
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("plugins: locate the user cache directory: %w", err)
	}
	return filepath.Join(cacheDirectory, "awake", "plugins"), nil
}

// Resolve returns the commit ref points at in the repository at url, fetching
// the repository first so a moved branch or tag is seen. An empty ref
// resolves the default branch.
func (cache *Cache) Resolve(operationContext context.Context, url string, ref string) (string, error) {
	if err := ValidateRepositoryURL(url); err != nil {
		return "", err
	}
	if err := validateRef(ref); err != nil {
		return "", fmt.Errorf("plugins: ref %w", err)
	}

	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	repository, err := cache.updatedRepository(operationContext, url)
	if err != nil {
		return "", err
	}
	commit, err := cache.git.ResolveCommit(operationContext, repository, ref)
	if err != nil {
		return "", fmt.Errorf("plugins: resolve %q in %s: %w", ref, url, err)
	}
	return commit, nil
}

// Checkout returns the directory holding the files of commit, extracting them
// first when they are not cached. The repository is only fetched when it
// lacks the commit.
func (cache *Cache) Checkout(operationContext context.Context, url string, commit string) (string, error) {
	if err := ValidateRepositoryURL(url); err != nil {
		return "", err
	}
	if !fullCommitHash.MatchString(commit) {
		return "", fmt.Errorf("%w: %q", ErrInvalidCommit, commit)
	}

	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	directory := cache.checkoutDirectory(url, commit)
	if isDirectory(directory) {
		return directory, nil
	}

	repository, err := cache.repositoryWithCommit(operationContext, url, commit)
	if err != nil {
		return "", err
	}
	if err := cache.extract(operationContext, repository, commit, directory); err != nil {
		return "", err
	}
	return directory, nil
}

// CachedCheckout returns the directory of a commit already extracted, without
// touching the network, so opening a workspace stays fast and works offline.
func (cache *Cache) CachedCheckout(url string, commit string) (string, bool) {
	if !fullCommitHash.MatchString(commit) {
		return "", false
	}
	directory := cache.checkoutDirectory(url, commit)
	return directory, isDirectory(directory)
}

// ReadFile reads a file of a checkout named by a manifest path. It refuses a
// path, or a symbolic link, that leads outside the checkout.
func ReadFile(checkout string, relative string) ([]byte, error) {
	resolved, err := resolvePluginFile(checkout, relative)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("plugins: read %s: %w", relative, err)
	}
	return content, nil
}

// ReadManifest reads and validates the manifest of a checkout.
func ReadManifest(checkout string) (Manifest, error) {
	content, err := ReadFile(checkout, ManifestFileName)
	if err != nil {
		return Manifest{}, err
	}
	return ParseManifest(content)
}

func (cache *Cache) updatedRepository(operationContext context.Context, url string) (string, error) {
	repository := cache.repositoryDirectory(url)
	if !isDirectory(repository) {
		return repository, cache.clone(operationContext, url, repository)
	}
	if err := cache.git.Fetch(operationContext, repository); err != nil {
		return "", fmt.Errorf("plugins: fetch %s: %w", url, err)
	}
	return repository, nil
}

func (cache *Cache) repositoryWithCommit(operationContext context.Context, url string, commit string) (string, error) {
	repository := cache.repositoryDirectory(url)
	if !isDirectory(repository) {
		if err := cache.clone(operationContext, url, repository); err != nil {
			return "", err
		}
	} else if found, err := cache.git.HasCommit(operationContext, repository, commit); err != nil {
		return "", fmt.Errorf("plugins: look for %s in %s: %w", commit, url, err)
	} else if !found {
		if err := cache.git.Fetch(operationContext, repository); err != nil {
			return "", fmt.Errorf("plugins: fetch %s: %w", url, err)
		}
	}

	found, err := cache.git.HasCommit(operationContext, repository, commit)
	if err != nil {
		return "", fmt.Errorf("plugins: look for %s in %s: %w", commit, url, err)
	}
	if !found {
		return "", fmt.Errorf("plugins: %s has no commit %s", url, commit)
	}
	return repository, nil
}

// clone clones into a temporary directory renamed once complete, so an
// interrupted clone never passes for a usable repository.
func (cache *Cache) clone(operationContext context.Context, url string, repository string) error {
	parent := filepath.Dir(repository)
	if err := os.MkdirAll(parent, checkoutDirectoryMode); err != nil {
		return fmt.Errorf("plugins: create %s: %w", parent, err)
	}
	staging, err := os.MkdirTemp(parent, filepath.Base(repository)+partialCheckoutSuffix)
	if err != nil {
		return fmt.Errorf("plugins: create a staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	target := filepath.Join(staging, filepath.Base(repository))
	if err := cache.git.CloneBare(operationContext, url, target); err != nil {
		return fmt.Errorf("plugins: clone %s: %w", url, err)
	}
	if err := os.Rename(target, repository); err != nil {
		return fmt.Errorf("plugins: store the clone of %s: %w", url, err)
	}
	return nil
}

// extract writes the files of commit into a staging directory renamed once
// complete. Symbolic links and other special entries are skipped: a plugin
// only needs regular files, and a link could point outside the checkout.
func (cache *Cache) extract(operationContext context.Context, repository string, commit string, directory string) error {
	parent := filepath.Dir(directory)
	if err := os.MkdirAll(parent, checkoutDirectoryMode); err != nil {
		return fmt.Errorf("plugins: create %s: %w", parent, err)
	}
	staging, err := os.MkdirTemp(parent, commit+partialCheckoutSuffix)
	if err != nil {
		return fmt.Errorf("plugins: create a staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	reader, writer := io.Pipe()
	archiveDone := make(chan error, 1)
	go func() {
		err := cache.git.Archive(operationContext, repository, commit, writer)
		writer.CloseWithError(err)
		archiveDone <- err
	}()

	extractError := extractTar(reader, staging)
	if extractError == nil {
		// git pads the archive past its end marker; read the padding so git
		// does not fail on a closed pipe.
		_, extractError = io.Copy(io.Discard, reader)
	}
	// Unblock the archive when extraction stopped early.
	reader.CloseWithError(errExtractionStopped)
	archiveError := <-archiveDone
	if extractError != nil {
		return extractError
	}
	if archiveError != nil {
		return fmt.Errorf("plugins: archive %s: %w", commit, archiveError)
	}

	if err := os.Rename(staging, directory); err != nil {
		return fmt.Errorf("plugins: store the files of %s: %w", commit, err)
	}
	return nil
}

func extractTar(source io.Reader, destination string) error {
	reader := tar.NewReader(source)
	var totalSize int64
	fileCount := 0

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("plugins: read the plugin archive: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeDir, tar.TypeReg:
		default:
			continue
		}

		relative := strings.TrimSuffix(header.Name, "/")
		if err := validateRelativePath(relative); err != nil {
			return fmt.Errorf("%w: %s", ErrUnsafePath, header.Name)
		}
		target := filepath.Join(destination, filepath.FromSlash(path.Clean(relative)))

		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, checkoutDirectoryMode); err != nil {
				return fmt.Errorf("plugins: create %s: %w", relative, err)
			}
			continue
		}

		fileCount++
		totalSize += header.Size
		if fileCount > maximumCheckoutFiles || totalSize > maximumCheckoutSize {
			return fmt.Errorf("%w: more than %d files or %d bytes", ErrCheckoutTooLarge, maximumCheckoutFiles, maximumCheckoutSize)
		}
		if err := writeArchivedFile(reader, target, header.Size); err != nil {
			return fmt.Errorf("plugins: write %s: %w", relative, err)
		}
	}
}

func writeArchivedFile(reader io.Reader, target string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(target), checkoutDirectoryMode); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, checkoutFileMode)
	if err != nil {
		return err
	}
	_, copyError := io.CopyN(file, reader, size)
	closeError := file.Close()
	return errors.Join(copyError, closeError)
}

// resolvePluginFile joins a manifest path to the checkout and checks that the
// result, once symbolic links are followed, is a regular file inside it.
func resolvePluginFile(checkout string, relative string) (string, error) {
	if err := validateRelativePath(relative); err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnsafePath, err)
	}

	root, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return "", fmt.Errorf("plugins: open checkout: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", fmt.Errorf("plugins: open %s: %w", relative, err)
	}
	inside, err := filepath.Rel(root, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrUnsafePath, relative)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("plugins: open %s: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("plugins: %s is not a file", relative)
	}
	return resolved, nil
}

func (cache *Cache) repositoryDirectory(url string) string {
	return filepath.Join(cache.root, repositoriesDirectoryName, repositoryKey(url)+bareRepositorySuffix)
}

func (cache *Cache) checkoutDirectory(url string, commit string) string {
	return filepath.Join(cache.root, checkoutsDirectoryName, repositoryKey(url), commit)
}

// repositoryKey names the cache entry of a URL with a short hash, which keeps
// paths short on Windows and free of characters a file name cannot hold.
func repositoryKey(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:repositoryKeyLength])
}

func isDirectory(directory string) bool {
	info, err := os.Stat(directory)
	return err == nil && info.IsDir()
}
