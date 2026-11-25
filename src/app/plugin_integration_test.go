package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/awake-client/awake/src/features/git"
	"github.com/awake-client/awake/src/features/plugins"
	"github.com/awake-client/awake/src/features/workspace"
)

// TestPluginsInstallFromGitRepositories runs the plugin service against real
// git repositories: a marketplace listing a plugin, both served over file://.
func TestPluginsInstallFromGitRepositories(t *testing.T) {
	isolateGit(t)
	sources := t.TempDir()

	pluginRepository := filepath.Join(sources, "example")
	commitFiles(t, pluginRepository, map[string]string{
		plugins.ManifestFileName: "version: 1\nid: example\nname: Example\npluginVersion: 1.0.0\nmain: dist/plugin.js\npermissions: [network]\n",
		"dist/plugin.js":         "awake.register({})\n",
	}, "v1.0.0")
	marketplaceRepository := filepath.Join(sources, "marketplace")
	commitFiles(t, marketplaceRepository, map[string]string{
		plugins.MarketplaceFileName: "version: 1\nname: local\nplugins:\n  - id: example\n    repository: file://" + filepath.ToSlash(pluginRepository) + "\n    ref: v1.0.0\n",
	}, "")

	workspaceRoot := t.TempDir()
	if _, err := workspace.Create(workspaceRoot, workspace.CreateOptions{Name: "Platform API", Mode: workspace.ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	cache := plugins.NewCache(t.TempDir(), git.BareRepositories{})
	service := plugins.NewService(cache, plugins.NewTrustStore(filepath.Join(t.TempDir(), "trust.yaml")), pluginWorkspace{}, "dev")

	if _, err := service.AddMarketplace(workspaceRoot, "file://"+filepath.ToSlash(marketplaceRepository)); err != nil {
		t.Fatalf("add marketplace: %v", err)
	}
	source := plugins.PluginSource{Marketplace: "local", PluginID: "example"}
	preview, err := service.Preview(workspaceRoot, source)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	installed, err := service.Install(workspaceRoot, source, preview.Commit)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if installed.State != plugins.PluginApproved || installed.Manifest == nil || installed.Manifest.Main != "dist/plugin.js" {
		t.Fatalf("unexpected installed plugin: %+v", installed)
	}

	configuration, err := workspace.NewRepository(workspaceRoot).Load()
	if err != nil {
		t.Fatalf("load awake.yaml: %v", err)
	}
	if len(configuration.Plugins) != 1 || configuration.Plugins[0].Commit != preview.Commit {
		t.Errorf("expected awake.yaml to pin the reviewed commit, got %+v", configuration.Plugins)
	}
}

func isolateGit(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_AUTHOR_NAME", "Awake Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@awake.local")
	t.Setenv("GIT_COMMITTER_NAME", "Awake Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@awake.local")
}

func commitFiles(t *testing.T, directory string, contents map[string]string, tag string) {
	t.Helper()

	runGit(t, "", "init", "--quiet", "--initial-branch=main", directory)
	for name, content := range contents {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, directory, "add", "--all")
	runGit(t, directory, "commit", "--quiet", "--message", "publish")
	if tag != "" {
		runGit(t, directory, "tag", tag)
	}
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()

	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}
