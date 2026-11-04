// Package app owns the application lifecycle: it creates the Wails
// application, wires the feature services and starts the main window.
package app

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	applicationName        = "Awake"
	applicationDescription = "Git-native API client"

	mainWindowTitle  = "Awake"
	mainWindowWidth  = 1280
	mainWindowHeight = 800
)

// Run creates the Wails application and blocks until the last window is
// closed.
func Run(frontendAssets embed.FS) error {
	featureServices := newServices()

	instance := application.New(application.Options{
		Name:        applicationName,
		Description: applicationDescription,
		Services: []application.Service{
			application.NewService(featureServices.Platform),
			application.NewService(featureServices.Workspace),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendAssets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	instance.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            mainWindowTitle,
		Width:            mainWindowWidth,
		Height:           mainWindowHeight,
		BackgroundColour: application.NewRGB(18, 18, 20),
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 38,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})

	return instance.Run()
}
