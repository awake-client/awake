package updates

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func gzippedTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressed)
	for name, content := range files {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestKeepArchiveEntryKeepsOnlyTheBinary(t *testing.T) {
	source := gzippedTarball(t, map[string]string{
		"awake":         "binary",
		"awake.png":     "icon",
		"awake.desktop": "entry",
	})

	var repacked bytes.Buffer
	if err := keepArchiveEntry(bytes.NewReader(source), &repacked, releaseBinaryName); err != nil {
		t.Fatalf("repack: %v", err)
	}

	decompressed, err := gzip.NewReader(&repacked)
	if err != nil {
		t.Fatal(err)
	}
	entries := tar.NewReader(decompressed)
	header, err := entries.Next()
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(entries)
	if header.Name != "awake" || string(content) != "binary" || header.Mode != 0o755 {
		t.Errorf("unexpected entry %q (%o): %q", header.Name, header.Mode, content)
	}
	if _, err := entries.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("expected a single entry, got %v", err)
	}
}

func TestKeepArchiveEntryRequiresTheBinary(t *testing.T) {
	source := gzippedTarball(t, map[string]string{"awake.png": "icon"})

	if err := keepArchiveEntry(bytes.NewReader(source), io.Discard, releaseBinaryName); err == nil {
		t.Error("expected an error for an archive without the binary")
	}
}

func TestGitHubChangelogReadsTheTag(t *testing.T) {
	var requested string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requested = request.URL.Path
		_, _ = writer.Write([]byte("# Changelog"))
	}))
	defer server.Close()

	source := GitHubChangelog{Repository: "owner/name", BaseURL: server.URL}
	content, err := source.Changelog(context.Background(), "v1.2.3")
	if err != nil {
		t.Fatalf("changelog: %v", err)
	}
	if requested != "/owner/name/v1.2.3/CHANGELOG.md" || content != "# Changelog" {
		t.Errorf("requested %q, got %q", requested, content)
	}
}

func TestGitHubChangelogReportsMissingFiles(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	source := GitHubChangelog{Repository: "owner/name", BaseURL: server.URL}
	if _, err := source.Changelog(context.Background(), "v1.2.3"); err == nil {
		t.Error("expected an error for a missing changelog")
	}
}
