package plugins

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/awake-client/awake/src/domain"
)

const (
	testWorkspaceRoot     = "/workspaces/platform"
	testMarketplaceURL    = "https://example.com/awake-plugins/marketplace.git"
	testMarketplaceName   = "example-market"
	marketplaceCommit     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	nextMarketplaceCommit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testAwakeVersion      = "0.11.0"
)

func TestServiceInstallsAPluginFromAMarketplace(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "v1.0.0", pluginManifest("example", "1.0.0", "network"))
	fixture.publishMarketplace(marketplaceCommit, "example", "v1.0.0")

	added, err := fixture.service.AddMarketplace(testWorkspaceRoot, testMarketplaceURL)
	if err != nil {
		t.Fatalf("add marketplace: %v", err)
	}
	if added.Name != testMarketplaceName || len(added.Plugins) != 1 || added.Plugins[0].Installed {
		t.Fatalf("unexpected marketplace: %+v", added)
	}

	source := PluginSource{Marketplace: testMarketplaceName, PluginID: "example"}
	preview, err := fixture.service.Preview(testWorkspaceRoot, source)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Commit != firstCommit || preview.Ref != "v1.0.0" || !preview.Manifest.HasPermission(PermissionNetwork) || preview.InstalledCommit != "" {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	installed, err := fixture.service.Install(testWorkspaceRoot, source, preview.Commit)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if installed.State != PluginApproved {
		t.Errorf("the user installing a plugin approves it, got %s", installed.State)
	}
	expected := domain.PluginDeclaration{
		ID: "example", Marketplace: testMarketplaceName, Repository: testRepositoryURL, Ref: "v1.0.0", Commit: firstCommit,
	}
	if declared := fixture.workspace.plugins; len(declared) != 1 || fmt.Sprint(declared[0]) != fmt.Sprint(expected) {
		t.Errorf("unexpected declarations: %+v", declared)
	}

	marketplaces, err := fixture.service.ListMarketplaces(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("list marketplaces: %v", err)
	}
	if !marketplaces[0].Plugins[0].Installed {
		t.Error("expected the catalog to show the plugin as installed")
	}
}

func TestServiceAsksATeammateToApproveAPulledPlugin(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "", pluginManifest("example", "1.0.0", "secrets"))
	fixture.workspace.plugins = []domain.PluginDeclaration{{ID: "example", Repository: testRepositoryURL, Commit: firstCommit}}

	views, err := fixture.service.ListPlugins(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if views[0].State != PluginNotDownloaded || views[0].Manifest != nil {
		t.Fatalf("expected a plugin not downloaded yet, got %+v", views[0])
	}
	if fixture.git.clones != 0 {
		t.Error("listing plugins must not touch the network")
	}

	downloaded, err := fixture.service.Download(testWorkspaceRoot, "example")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if downloaded.State != PluginNeedsApproval || !downloaded.Manifest.HasPermission(PermissionSecrets) {
		t.Fatalf("expected the plugin to wait for approval, got %+v", downloaded)
	}

	if _, err := fixture.service.Approve(testWorkspaceRoot, "example", secondCommit); !errors.Is(err, ErrDeclarationChanged) {
		t.Fatalf("expected ErrDeclarationChanged for another commit, got %v", err)
	}
	approved, err := fixture.service.Approve(testWorkspaceRoot, "example", firstCommit)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.State != PluginApproved {
		t.Errorf("expected the plugin to be approved, got %s", approved.State)
	}

	// A pull moves the plugin to a commit asking for a new permission.
	fixture.publishPlugin(secondCommit, "", pluginManifest("example", "1.1.0", "secrets", "network"))
	fixture.workspace.plugins[0].Commit = secondCommit
	if _, err := fixture.service.Download(testWorkspaceRoot, "example"); err != nil {
		t.Fatalf("download the new commit: %v", err)
	}
	views, err = fixture.service.ListPlugins(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("list after pull: %v", err)
	}
	if views[0].State != PluginNeedsApproval {
		t.Errorf("a new commit must be approved again, got %s", views[0].State)
	}
}

func TestServiceUpdatesAPluginToTheRefItsMarketplaceListsToday(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "v1.0.0", pluginManifestWithSettings("example", "1.0.0"))
	fixture.publishMarketplace(marketplaceCommit, "example", "v1.0.0")
	if _, err := fixture.service.AddMarketplace(testWorkspaceRoot, testMarketplaceURL); err != nil {
		t.Fatalf("add marketplace: %v", err)
	}
	source := PluginSource{Marketplace: testMarketplaceName, PluginID: "example"}
	if _, err := fixture.service.Install(testWorkspaceRoot, source, firstCommit); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := fixture.service.UpdateSettings(testWorkspaceRoot, "example", map[string]string{"region": "us-east-1", "verbose": "true"}); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	update, err := fixture.service.CheckUpdate(testWorkspaceRoot, "example")
	if err != nil || update != nil {
		t.Fatalf("expected no update yet, got %+v: %v", update, err)
	}

	fixture.publishPlugin(secondCommit, "v2.0.0", pluginManifest("example", "2.0.0"))
	fixture.publishMarketplace(nextMarketplaceCommit, "example", "v2.0.0")
	update, err = fixture.service.CheckUpdate(testWorkspaceRoot, "example")
	if err != nil || update == nil {
		t.Fatalf("expected an update, got %+v: %v", update, err)
	}
	if update.Commit != secondCommit || update.InstalledCommit != firstCommit || update.Ref != "v2.0.0" {
		t.Fatalf("unexpected update: %+v", update)
	}

	if _, err := fixture.service.Install(testWorkspaceRoot, update.Source, update.Commit); err != nil {
		t.Fatalf("install the update: %v", err)
	}
	declaration := fixture.workspace.plugins[0]
	if declaration.Commit != secondCommit || declaration.Ref != "v2.0.0" {
		t.Errorf("unexpected declaration after update: %+v", declaration)
	}
	if declaration.Settings != nil {
		t.Errorf("settings the new manifest no longer declares must be dropped, got %v", declaration.Settings)
	}
}

func TestServiceInstallsAPluginFromItsRepository(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "v1.0.0", pluginManifest("example", "1.0.0"))
	source := PluginSource{Repository: testRepositoryURL, Ref: "v1.0.0"}

	preview, err := fixture.service.Preview(testWorkspaceRoot, source)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := fixture.service.Install(testWorkspaceRoot, source, preview.Commit); err != nil {
		t.Fatalf("install: %v", err)
	}

	declaration := fixture.workspace.plugins[0]
	if declaration.ID != "example" || declaration.Marketplace != "" || declaration.Ref != "v1.0.0" {
		t.Errorf("unexpected declaration: %+v", declaration)
	}
	if update, err := fixture.service.CheckUpdate(testWorkspaceRoot, "example"); err != nil || update != nil {
		t.Errorf("expected no update, got %+v: %v", update, err)
	}
}

func TestServiceRefusesPluginsItCannotRun(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		expected error
	}{
		{"another plugin", pluginManifest("impostor", "1.0.0"), ErrPluginMismatch},
		{"newer engine", strings.Replace(pluginManifest("example", "1.0.0"), "main:", "engine: \">=9.0.0\"\nmain:", 1), ErrIncompatiblePlugin},
		{"invalid manifest", "version: 1\nid: example\n", ErrInvalidManifest},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixture(t)
			fixture.publishPlugin(firstCommit, "v1.0.0", test.manifest)
			fixture.publishMarketplace(marketplaceCommit, "example", "v1.0.0")
			if _, err := fixture.service.AddMarketplace(testWorkspaceRoot, testMarketplaceURL); err != nil {
				t.Fatalf("add marketplace: %v", err)
			}

			_, err := fixture.service.Preview(testWorkspaceRoot, PluginSource{Marketplace: testMarketplaceName, PluginID: "example"})

			if !errors.Is(err, test.expected) {
				t.Fatalf("expected %v, got %v", test.expected, err)
			}
		})
	}
}

func TestServiceReportsDeclaredPluginsItCannotRun(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "", pluginManifest("impostor", "1.0.0"))
	fixture.publishPlugin(secondCommit, "", strings.Replace(pluginManifest("example", "1.0.0"), "main:", "engine: \">=9.0.0\"\nmain:", 1))
	fixture.workspace.plugins = []domain.PluginDeclaration{
		{ID: "example", Repository: testRepositoryURL, Commit: firstCommit},
	}
	if _, err := fixture.service.Download(testWorkspaceRoot, "example"); err != nil {
		t.Fatalf("download: %v", err)
	}
	views, err := fixture.service.ListPlugins(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if views[0].State != PluginInvalid || !strings.Contains(views[0].Problem, "another plugin") {
		t.Errorf("expected a mismatched plugin to be invalid, got %+v", views[0])
	}
	if _, err := fixture.service.Approve(testWorkspaceRoot, "example", firstCommit); !errors.Is(err, ErrPluginMismatch) {
		t.Errorf("expected approving a mismatched plugin to fail, got %v", err)
	}

	fixture.workspace.plugins[0].Commit = secondCommit
	if _, err := fixture.service.Download(testWorkspaceRoot, "example"); err != nil {
		t.Fatalf("download: %v", err)
	}
	views, err = fixture.service.ListPlugins(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if views[0].State != PluginIncompatible {
		t.Errorf("expected an incompatible plugin, got %+v", views[0])
	}
}

func TestServiceValidatesSettings(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "", pluginManifestWithSettings("example", "1.0.0"))
	if _, err := fixture.service.Install(testWorkspaceRoot, PluginSource{Repository: testRepositoryURL}, firstCommit); err != nil {
		t.Fatalf("install: %v", err)
	}

	refused := []map[string]string{
		{"unknown": "value"},
		{"token": "secret value"},
		{"region": "mars-north-1"},
		{"verbose": "yes"},
	}
	for _, settings := range refused {
		if _, err := fixture.service.UpdateSettings(testWorkspaceRoot, "example", settings); !errors.Is(err, ErrInvalidSetting) {
			t.Errorf("expected %v to be refused, got %v", settings, err)
		}
	}

	view, err := fixture.service.UpdateSettings(testWorkspaceRoot, "example", map[string]string{"region": "us-east-1"})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
	if view.Declaration.Settings["region"] != "us-east-1" || fixture.workspace.plugins[0].Settings["region"] != "us-east-1" {
		t.Errorf("expected the setting to be saved, got %+v", fixture.workspace.plugins[0])
	}
	if _, err := fixture.service.UpdateSettings(testWorkspaceRoot, "example", map[string]string{}); err != nil {
		t.Fatalf("clear settings: %v", err)
	}
	if fixture.workspace.plugins[0].Settings != nil {
		t.Errorf("expected cleared settings to be removed, got %v", fixture.workspace.plugins[0].Settings)
	}
}

func TestServiceRevokesAndUninstallsPlugins(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "", pluginManifest("example", "1.0.0"))
	if _, err := fixture.service.Install(testWorkspaceRoot, PluginSource{Repository: testRepositoryURL}, firstCommit); err != nil {
		t.Fatalf("install: %v", err)
	}

	if err := fixture.service.Revoke(testWorkspaceRoot, "example"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	views, err := fixture.service.ListPlugins(testWorkspaceRoot)
	if err != nil || len(views) != 1 || views[0].State != PluginNeedsApproval {
		t.Fatalf("a revoked plugin stays declared and waits for approval, got %+v: %v", views, err)
	}

	if _, err := fixture.service.Approve(testWorkspaceRoot, "example", firstCommit); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := fixture.service.Uninstall(testWorkspaceRoot, "example"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if len(fixture.workspace.plugins) != 0 {
		t.Errorf("expected no declaration left, got %+v", fixture.workspace.plugins)
	}
	if trusted, _ := fixture.trust.IsTrusted(testWorkspaceID, "example", firstCommit, nil); trusted {
		t.Error("uninstalling must forget the approval")
	}
	if err := fixture.service.Uninstall(testWorkspaceRoot, "example"); !errors.Is(err, ErrPluginNotDeclared) {
		t.Errorf("expected ErrPluginNotDeclared, got %v", err)
	}
}

func TestServiceManagesMarketplaces(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.publishPlugin(firstCommit, "v1.0.0", pluginManifest("example", "1.0.0"))
	fixture.publishMarketplace(marketplaceCommit, "example", "v1.0.0")

	if _, err := fixture.service.AddMarketplace(testWorkspaceRoot, testMarketplaceURL); err != nil {
		t.Fatalf("add marketplace: %v", err)
	}
	if _, err := fixture.service.AddMarketplace(testWorkspaceRoot, testMarketplaceURL); !errors.Is(err, ErrMarketplaceExists) {
		t.Errorf("expected ErrMarketplaceExists, got %v", err)
	}

	fixture.publishMarketplace(nextMarketplaceCommit, "example", "v1.0.0", "second")
	fixture.git.offline = true
	refreshed, err := fixture.service.RefreshMarketplaces(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed[0].Problem == "" || len(refreshed[0].Plugins) != 1 {
		t.Errorf("an offline refresh keeps the last catalog and reports the problem, got %+v", refreshed[0])
	}

	fixture.git.offline = false
	refreshed, err = fixture.service.RefreshMarketplaces(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed[0].Problem != "" || len(refreshed[0].Plugins) != 2 {
		t.Errorf("expected the refreshed catalog, got %+v", refreshed[0])
	}

	if err := fixture.service.RemoveMarketplace(testWorkspaceRoot, testMarketplaceName); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(fixture.workspace.marketplaces) != 0 {
		t.Errorf("expected no marketplace left, got %+v", fixture.workspace.marketplaces)
	}
	if err := fixture.service.RemoveMarketplace(testWorkspaceRoot, testMarketplaceName); !errors.Is(err, ErrMarketplaceNotDeclared) {
		t.Errorf("expected ErrMarketplaceNotDeclared, got %v", err)
	}
}

func TestServiceListsAMarketplaceNeverDownloaded(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.workspace.marketplaces = []domain.MarketplaceReference{{Name: testMarketplaceName, Repository: testMarketplaceURL}}

	views, err := fixture.service.ListMarketplaces(testWorkspaceRoot)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(views[0].Problem, "not downloaded") || views[0].Plugins == nil {
		t.Errorf("expected an empty catalog with a problem, got %+v", views[0])
	}
}

type serviceFixture struct {
	t         *testing.T
	git       *fakeGit
	workspace *fakeWorkspace
	trust     *TrustStore
	service   *Service
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()

	git := newFakeGit()
	workspace := &fakeWorkspace{}
	trust := NewTrustStore(filepath.Join(t.TempDir(), trustFileName))
	service := NewService(NewCache(t.TempDir(), git), trust, workspace, testAwakeVersion)
	return &serviceFixture{t: t, git: git, workspace: workspace, trust: trust, service: service}
}

func (fixture *serviceFixture) publishPlugin(commit string, tag string, manifest string) {
	fixture.t.Helper()
	fixture.git.publishTo(testRepositoryURL, commit, tag, testArchive(fixture.t, map[string]string{
		ManifestFileName: manifest,
		"dist/plugin.js": "awake.register({})\n",
	}))
}

// publishMarketplace lists the test plugin at ref, plus extra plugins that
// point at other repositories.
func (fixture *serviceFixture) publishMarketplace(commit string, pluginID string, ref string, extraIDs ...string) {
	fixture.t.Helper()
	index := fmt.Sprintf("version: 1\nname: %s\nplugins:\n  - id: %s\n    repository: %s\n    ref: %s\n", testMarketplaceName, pluginID, testRepositoryURL, ref)
	for _, id := range extraIDs {
		index += fmt.Sprintf("  - id: %s\n    repository: https://example.com/%s.git\n", id, id)
	}
	fixture.git.publishTo(testMarketplaceURL, commit, "", testArchive(fixture.t, map[string]string{MarketplaceFileName: index}))
}

func pluginManifest(id string, version string, permissions ...string) string {
	manifest := fmt.Sprintf("version: 1\nid: %s\nname: Example\npluginVersion: %s\nmain: dist/plugin.js\n", id, version)
	if len(permissions) > 0 {
		manifest += "permissions: [" + strings.Join(permissions, ", ") + "]\n"
	}
	return manifest
}

func pluginManifestWithSettings(id string, version string) string {
	return pluginManifest(id, version) + `settings:
  - { key: region, type: select, label: Region, options: [eu-west-3, us-east-1] }
  - { key: verbose, type: boolean, label: Verbose }
  - { key: token, type: secret, label: Token }
`
}

// fakeWorkspace holds the plugin section of an awake.yaml in memory.
type fakeWorkspace struct {
	marketplaces []domain.MarketplaceReference
	plugins      []domain.PluginDeclaration
}

func (workspace *fakeWorkspace) LoadPlugins(workspaceRoot string) (WorkspacePlugins, error) {
	if workspaceRoot != testWorkspaceRoot {
		return WorkspacePlugins{}, fmt.Errorf("unknown workspace %s", workspaceRoot)
	}
	return WorkspacePlugins{
		WorkspaceID:  testWorkspaceID,
		Marketplaces: slices.Clone(workspace.marketplaces),
		Plugins:      slices.Clone(workspace.plugins),
	}, nil
}

func (workspace *fakeWorkspace) SavePlugins(_ string, marketplaces []domain.MarketplaceReference, plugins []domain.PluginDeclaration) error {
	workspace.marketplaces = slices.Clone(marketplaces)
	workspace.plugins = slices.Clone(plugins)
	return nil
}
