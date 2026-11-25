package app

import (
	"testing"

	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/workspace"
)

func TestPluginWorkspaceKeepsTheRestOfTheConfiguration(t *testing.T) {
	rootPath := t.TempDir()
	created, err := workspace.Create(rootPath, workspace.CreateOptions{Name: "Platform API", Mode: workspace.ModeTeam})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := workspace.NewService().UpdateColor(rootPath, domain.Colors[0]); err != nil {
		t.Fatalf("update color: %v", err)
	}

	adapter := pluginWorkspace{}
	marketplaces := []domain.MarketplaceReference{{Name: "awake-official", Repository: "https://example.com/marketplace.git"}}
	declarations := []domain.PluginDeclaration{{ID: "example", Repository: "https://example.com/example.git", Commit: "1111111111111111111111111111111111111111"}}
	if err := adapter.SavePlugins(rootPath, marketplaces, declarations); err != nil {
		t.Fatalf("save plugins: %v", err)
	}

	loaded, err := adapter.LoadPlugins(rootPath)
	if err != nil {
		t.Fatalf("load plugins: %v", err)
	}
	if loaded.WorkspaceID != created.ID || len(loaded.Marketplaces) != 1 || len(loaded.Plugins) != 1 {
		t.Errorf("unexpected plugins: %+v", loaded)
	}
	configuration, err := workspace.NewRepository(rootPath).Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if configuration.Color != domain.Colors[0] || configuration.Name != "Platform API" {
		t.Errorf("saving plugins must keep the rest of awake.yaml, got %+v", configuration)
	}
}
