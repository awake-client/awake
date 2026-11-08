package updates

import (
	"errors"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func releaseAssets(names ...string) []github.ReleaseAsset {
	assets := make([]github.ReleaseAsset, len(names))
	for index, name := range names {
		assets[index] = github.ReleaseAsset{Name: name}
	}
	return assets
}

func assetName(t *testing.T, platform, arch string, assets []github.ReleaseAsset) string {
	t.Helper()
	index := matchReleaseAsset(updater.CheckRequest{Platform: platform, Arch: arch}, assets)
	if index < 0 || index >= len(assets) {
		return ""
	}
	return assets[index].Name
}

func TestMatchReleaseAssetFindsThePlatformArchive(t *testing.T) {
	assets := releaseAssets(
		"awake-linux-amd64.tar.gz",
		"awake-linux-arm64.tar.gz",
		"awake-macos-amd64.zip",
		"awake-macos-arm64.zip",
		"awake-windows-amd64.zip",
		"SHA256SUMS",
	)

	cases := []struct {
		platform string
		arch     string
		want     string
	}{
		{"darwin", "arm64", "awake-macos-arm64.zip"},
		{"darwin", "amd64", "awake-macos-amd64.zip"},
		{"linux", "amd64", "awake-linux-amd64.tar.gz"},
		{"linux", "arm64", "awake-linux-arm64.tar.gz"},
		{"windows", "amd64", "awake-windows-amd64.zip"},
	}
	for _, test := range cases {
		if got := assetName(t, test.platform, test.arch, assets); got != test.want {
			t.Errorf("%s/%s: got %q, want %q", test.platform, test.arch, got, test.want)
		}
	}
}

func TestMatchReleaseAssetAcceptsDarwinNames(t *testing.T) {
	assets := releaseAssets("awake-darwin-arm64.zip", "awake-darwin-amd64.zip")

	if got := assetName(t, "darwin", "arm64", assets); got != "awake-darwin-arm64.zip" {
		t.Errorf("got %q, want the darwin archive", got)
	}
}

func TestMatchReleaseAssetRejectsAnUnknownArch(t *testing.T) {
	assets := releaseAssets("awake-macos-arm64.zip", "awake-macos-amd64.zip")

	if got := assetName(t, "darwin", "386", assets); got != "" {
		t.Errorf("got %q, want no asset", got)
	}
}

func TestReadableProviderErrorHidesAnHTMLPage(t *testing.T) {
	err := readableProviderError(errors.New(
		"github: api 504: <!DOCTYPE html><html><body>Unicorn!</body></html>",
	))

	if strings.Contains(err.Error(), "DOCTYPE") || strings.Contains(err.Error(), "<html") {
		t.Errorf("the HTML page is still shown: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 504") {
		t.Errorf("the status is missing: %v", err)
	}
}

func TestReadableProviderErrorKeepsOtherErrors(t *testing.T) {
	original := errors.New("github: release v0.0.6 has no asset for darwin/arm64")

	if got := readableProviderError(original); got != original {
		t.Errorf("got %v, want the original error", got)
	}
}
