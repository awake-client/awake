package app

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/awake-client/awake/src/features/updates"
)

const updateRepository = "awake-client/awake"

// configureUpdates connects the update service to the Wails updater and the
// GitHub releases of the project.
func configureUpdates(instance *application.App, service *updates.Service) error {
	provider, err := updates.NewGitHubProvider(updateRepository)
	if err != nil {
		return fmt.Errorf("configure update provider: %w", err)
	}

	if err := instance.Updater.Init(updater.Config{
		CurrentVersion: updates.Version,
		Providers:      []updater.Provider{provider},
		Window:         &updater.BuiltinWindow{},
	}); err != nil {
		return fmt.Errorf("configure updater: %w", err)
	}

	updates.Use(service, instance.Updater)
	return nil
}
