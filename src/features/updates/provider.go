package updates

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

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

func (provider *GitHubProvider) Download(ctx context.Context, release *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return provider.provider.Download(ctx, release, dst, onProgress)
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
