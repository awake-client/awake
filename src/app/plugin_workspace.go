package app

import (
	"github.com/awake-client/awake/src/domain"
	"github.com/awake-client/awake/src/features/plugins"
	"github.com/awake-client/awake/src/features/workspace"
)

// pluginWorkspace connects the plugins feature to awake.yaml, which the
// workspace feature owns.
type pluginWorkspace struct{}

func (pluginWorkspace) LoadPlugins(workspaceRoot string) (plugins.WorkspacePlugins, error) {
	configuration, err := workspace.NewRepository(workspaceRoot).Load()
	if err != nil {
		return plugins.WorkspacePlugins{}, err
	}
	return plugins.WorkspacePlugins{
		WorkspaceID:  configuration.ID,
		Marketplaces: configuration.Marketplaces,
		Plugins:      configuration.Plugins,
	}, nil
}

// SavePlugins reloads awake.yaml before writing it, so a setting changed
// meanwhile, such as the color, is kept.
func (pluginWorkspace) SavePlugins(workspaceRoot string, marketplaces []domain.MarketplaceReference, declarations []domain.PluginDeclaration) error {
	repository := workspace.NewRepository(workspaceRoot)
	configuration, err := repository.Load()
	if err != nil {
		return err
	}
	configuration.Marketplaces = marketplaces
	configuration.Plugins = declarations
	return repository.Save(configuration)
}
