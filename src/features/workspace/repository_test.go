package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
)

func TestCreateThenOpen(t *testing.T) {
	rootPath := t.TempDir()

	created, err := Create(rootPath, CreateOptions{
		Name: "Platform API",
		Mode: ModeTeam,
		Git: &GitConfiguration{
			Remote:                  "origin",
			Branch:                  "main",
			AutomaticPull:           true,
			AutomaticPush:           true,
			SynchronizationInterval: "5m",
		},
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if !strings.HasPrefix(created.ID, "workspace_") {
		t.Errorf("unexpected identifier %q", created.ID)
	}

	opened, err := NewService().Open(rootPath)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}

	if opened.ID != created.ID || opened.Name != "Platform API" || opened.Mode != ModeTeam {
		t.Errorf("opened configuration does not match created configuration: %+v", opened)
	}

	for _, directory := range []string{"collections", "environments", "local", ".awake"} {
		path := filepath.Join(rootPath, directory)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected directory %s: %v", path, err)
		}
	}

	ignore, err := os.ReadFile(filepath.Join(rootPath, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, entry := range ignoreEntries {
		if !strings.Contains(string(ignore), entry) {
			t.Errorf("expected .gitignore to contain %q:\n%s", entry, ignore)
		}
	}
}

func TestCreateKeepsExistingIgnoreEntries(t *testing.T) {
	rootPath := t.TempDir()
	ignorePath := filepath.Join(rootPath, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("custom-entry\nlocal/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	if _, err := Create(rootPath, CreateOptions{Name: "Personal", Mode: ModePersonal}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	content, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "custom-entry") {
		t.Errorf("existing entry was lost:\n%s", text)
	}
	if strings.Count(text, "local/") != 1 {
		t.Errorf("local/ should appear once:\n%s", text)
	}
}

func TestCreateRejectsExistingWorkspace(t *testing.T) {
	rootPath := t.TempDir()

	if _, err := Create(rootPath, CreateOptions{Name: "One", Mode: ModePersonal}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := Create(rootPath, CreateOptions{Name: "Two", Mode: ModePersonal})
	if !errors.Is(err, ErrWorkspaceExists) {
		t.Fatalf("expected ErrWorkspaceExists, got %v", err)
	}
}

func TestCreateRejectsInvalidOptions(t *testing.T) {
	testCases := []struct {
		name    string
		options CreateOptions
	}{
		{name: "empty name", options: CreateOptions{Name: "  ", Mode: ModePersonal}},
		{name: "unknown mode", options: CreateOptions{Name: "Test", Mode: Mode("shared")}},
		{
			name: "invalid interval",
			options: CreateOptions{
				Name: "Test",
				Mode: ModeTeam,
				Git:  &GitConfiguration{SynchronizationInterval: "soon"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Create(t.TempDir(), testCase.options)
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("expected ErrInvalidConfiguration, got %v", err)
			}
		})
	}
}

func TestOpenRejectsDirectoryWithoutConfiguration(t *testing.T) {
	_, err := NewService().Open(t.TempDir())
	if !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestOpenRejectsNewerFormatVersion(t *testing.T) {
	rootPath := t.TempDir()
	content := "version: 99\nid: workspace_test\nname: Future\nmode: team\n"
	if err := os.WriteFile(filepath.Join(rootPath, "awake.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write configuration: %v", err)
	}

	_, err := NewRepository(rootPath).Load()
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestSaveWritesCanonicalYaml(t *testing.T) {
	rootPath := t.TempDir()
	configuration, err := Create(rootPath, CreateOptions{Name: "Canonical", Mode: ModeTeam})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(rootPath, "awake.yaml"))
	if err != nil {
		t.Fatalf("read configuration: %v", err)
	}

	text := string(content)
	if !strings.HasPrefix(text, "version: 1\nid: workspace_") {
		t.Errorf("unexpected field order:\n%s", text)
	}
	if strings.Contains(text, "  id:") {
		t.Errorf("unexpected indentation:\n%s", text)
	}

	configuration.Name = "Renamed"
	if err := NewRepository(rootPath).Save(configuration); err != nil {
		t.Fatalf("save configuration: %v", err)
	}

	opened, err := NewRepository(rootPath).Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if opened.Name != "Renamed" {
		t.Errorf("expected Renamed, got %q", opened.Name)
	}
}

func TestUpdateGitStoresSynchronizationSettings(t *testing.T) {
	rootPath := t.TempDir()
	service := NewService()
	if _, err := service.Create(rootPath, CreateOptions{Name: "Team", Mode: ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	updated, err := service.UpdateGit(rootPath, GitConfiguration{
		AutomaticCommit:         true,
		AutomaticPull:           true,
		AutomaticPush:           true,
		SynchronizationInterval: "2m",
	})
	if err != nil {
		t.Fatalf("update git settings: %v", err)
	}
	if updated.Git == nil || !updated.Git.AutomaticCommit || !updated.Git.AutomaticPull || updated.Git.SynchronizationInterval != "2m" {
		t.Fatalf("unexpected configuration: %+v", updated.Git)
	}

	reloaded, err := service.Open(rootPath)
	if err != nil {
		t.Fatalf("reopen workspace: %v", err)
	}
	if reloaded.Git == nil || !reloaded.Git.AutomaticPush {
		t.Fatalf("settings were not persisted: %+v", reloaded.Git)
	}

	if _, err := service.UpdateGit(rootPath, GitConfiguration{SynchronizationInterval: "soon"}); err == nil {
		t.Fatal("expected an invalid interval to be rejected")
	}
}

func TestUpdateColorAcceptsOnlyKnownColors(t *testing.T) {
	rootPath := t.TempDir()
	service := NewService()
	if _, err := service.Create(rootPath, CreateOptions{Name: "Colors", Mode: ModeTeam}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	updated, err := service.UpdateColor(rootPath, "teal")
	if err != nil || updated.Color != "teal" {
		t.Fatalf("expected teal, got %q, %v", updated.Color, err)
	}
	reopened, err := service.Open(rootPath)
	if err != nil || reopened.Color != "teal" {
		t.Fatalf("the color must be persisted, got %q, %v", reopened.Color, err)
	}
	if _, err := service.UpdateColor(rootPath, "#ff0000"); err == nil {
		t.Fatal("expected an unknown color to be rejected")
	}
	if cleared, err := service.UpdateColor(rootPath, ""); err != nil || cleared.Color != "" {
		t.Fatalf("an empty color removes it, got %q, %v", cleared.Color, err)
	}
}

func TestSaveKeepsMarketplacesAndPlugins(t *testing.T) {
	rootPath := t.TempDir()
	created, err := Create(rootPath, CreateOptions{Name: "Platform API", Mode: ModeTeam})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	created.Marketplaces = []domain.MarketplaceReference{
		{Name: "awake-official", Repository: "https://github.com/awake-client/marketplace.git"},
	}
	created.Plugins = []domain.PluginDeclaration{{
		ID:          "aws-sigv4",
		Marketplace: "awake-official",
		Repository:  "https://github.com/awake-plugins/aws-sigv4.git",
		Ref:         "v1.2.0",
		Commit:      "3f2a9c1e0b7d4a5f6e8c9d0a1b2c3d4e5f6a7b8c",
		Settings:    map[string]string{"region": "eu-west-3"},
	}}
	repository := NewRepository(rootPath)
	if err := repository.Save(created); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := repository.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Marketplaces) != 1 || loaded.Marketplaces[0].Name != "awake-official" {
		t.Errorf("unexpected marketplaces: %+v", loaded.Marketplaces)
	}
	if len(loaded.Plugins) != 1 || loaded.Plugins[0].Commit != created.Plugins[0].Commit || loaded.Plugins[0].Settings["region"] != "eu-west-3" {
		t.Errorf("unexpected plugins: %+v", loaded.Plugins)
	}

	content, err := os.ReadFile(repository.ConfigurationPath())
	if err != nil {
		t.Fatalf("read awake.yaml: %v", err)
	}
	for _, expected := range []string{"marketplaces:\n  - {name: awake-official, ", "plugins:\n  - id: aws-sigv4\n"} {
		if !strings.Contains(string(content), expected) {
			t.Errorf("expected %q in awake.yaml:\n%s", expected, content)
		}
	}
}
