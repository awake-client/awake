package updates

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// platformAliases lists the other names a release asset may use for a GOOS.
// The updater reports darwin, while the project used to publish macOS
// archives as awake-macos-<arch>.
var platformAliases = map[string][]string{
	"darwin": {"macos"},
}

// NewGitHubProvider returns the GitHub releases provider of the updater. It
// recognizes the asset names this project publishes and reports an outage
// GitHub answers with an HTML page as a readable error.
func NewGitHubProvider(repository string) (*GitHubProvider, error) {
	provider, err := github.New(github.Config{
		Repository:   repository,
		AssetMatcher: matchReleaseAsset,
	})
	if err != nil {
		return nil, err
	}
	return &GitHubProvider{provider: provider}, nil
}

// GitHubProvider adapts the Wails GitHub provider to this project.
type GitHubProvider struct {
	provider *github.Provider
}

func (provider *GitHubProvider) Name() string {
	return provider.provider.Name()
}

func (provider *GitHubProvider) Check(ctx context.Context, request updater.CheckRequest) (*updater.Release, error) {
	release, err := provider.provider.Check(ctx, request)
	if err != nil {
		return nil, readableProviderError(err)
	}
	return release, nil
}

// Download streams the release archive. A tarball, which is how Linux
// releases ship, also carries the icon and desktop entry install.sh uses; the
// updater only swaps a single entry, so the tarball is rewritten with the
// binary alone.
func (provider *GitHubProvider) Download(ctx context.Context, release *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	if !strings.HasSuffix(strings.ToLower(release.Artifact.Filename), ".tar.gz") {
		return provider.provider.Download(ctx, release, dst, onProgress)
	}

	archive, err := os.CreateTemp("", "awake-update-*.tar.gz")
	if err != nil {
		return fmt.Errorf("buffer the update: %w", err)
	}
	defer os.Remove(archive.Name())
	defer archive.Close()

	if err := provider.provider.Download(ctx, release, archive, onProgress); err != nil {
		return err
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("read the update: %w", err)
	}
	return keepArchiveEntry(archive, dst, releaseBinaryName)
}

// releaseBinaryName is the name of the executable inside a release archive.
const releaseBinaryName = "awake"

// keepArchiveEntry copies the regular file called name from the gzipped
// tarball source into a new gzipped tarball written to destination.
func keepArchiveEntry(source io.Reader, destination io.Writer, name string) error {
	decompressed, err := gzip.NewReader(source)
	if err != nil {
		return fmt.Errorf("read the update archive: %w", err)
	}
	defer decompressed.Close()

	compressed := gzip.NewWriter(destination)
	archive := tar.NewWriter(compressed)
	entries := tar.NewReader(decompressed)
	found := false
	for {
		header, err := entries.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read the update archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || path.Clean(header.Name) != name {
			continue
		}
		if err := archive.WriteHeader(header); err != nil {
			return fmt.Errorf("repack the update: %w", err)
		}
		if _, err := io.Copy(archive, entries); err != nil {
			return fmt.Errorf("repack the update: %w", err)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("the update archive has no %s executable", name)
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("repack the update: %w", err)
	}
	if err := compressed.Close(); err != nil {
		return fmt.Errorf("repack the update: %w", err)
	}
	return nil
}

const (
	changelogTimeout     = 15 * time.Second
	maximumChangelogSize = 1 << 20
	changelogFile        = "CHANGELOG.md"
)

// GitHubChangelog reads the changelog of the repository as it was at a
// release tag.
type GitHubChangelog struct {
	Repository string
	// BaseURL serves raw repository files; empty means GitHub.
	BaseURL string
	Client  *http.Client
}

// Changelog returns the CHANGELOG.md file of the tag.
func (source GitHubChangelog) Changelog(ctx context.Context, tag string) (string, error) {
	baseURL := source.BaseURL
	if baseURL == "" {
		baseURL = "https://raw.githubusercontent.com"
	}
	client := source.Client
	if client == nil {
		client = &http.Client{Timeout: changelogTimeout}
	}

	address := strings.TrimRight(baseURL, "/") + "/" + source.Repository + "/" + tag + "/" + changelogFile
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", fmt.Errorf("changelog: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("changelog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("changelog: HTTP %d", response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maximumChangelogSize))
	if err != nil {
		return "", fmt.Errorf("changelog: %w", err)
	}
	return string(content), nil
}

// matchReleaseAsset picks the asset of the running platform. The default
// matcher is tried first, then the aliases of the platform, so a release
// keeps working whichever name it uses.
func matchReleaseAsset(request updater.CheckRequest, assets []github.ReleaseAsset) int {
	if index := github.DefaultAssetMatcher(request, assets); index >= 0 {
		return index
	}
	for _, alias := range platformAliases[request.Platform] {
		aliased := request
		aliased.Platform = alias
		if index := github.DefaultAssetMatcher(aliased, assets); index >= 0 {
			return index
		}
	}
	return -1
}

var apiStatus = regexp.MustCompile(`^github: api (\d{3}):`)

// readableProviderError replaces the HTML page GitHub sends during an outage
// with a message that fits in the update window.
func readableProviderError(err error) error {
	message := err.Error()
	lower := strings.ToLower(message)
	if !strings.Contains(lower, "<!doctype html") && !strings.Contains(lower, "<html") {
		return err
	}
	if match := apiStatus.FindStringSubmatch(message); len(match) == 2 {
		return fmt.Errorf(
			"github: the release API is unavailable right now (HTTP %s); try again in a moment",
			match[1],
		)
	}
	return fmt.Errorf("github: the release API returned an unexpected page; try again in a moment")
}
