// Package app owns the application lifecycle: it creates the Wails
// application, wires the feature services and starts the main window.
package app

import (
	"embed"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	applicationName        = "Awake"
	applicationDescription = "Git-native API client"

	mainWindowTitle  = "Awake"
	mainWindowWidth  = 1280
	mainWindowHeight = 800
	// Keep the sidebar and the request pane, with its response toolbar, usable.
	mainWindowMinWidth = 900
)

// Run creates the Wails application and blocks until the last window is
// closed.
func Run(frontendAssets embed.FS) error {
	featureServices, err := newServices()
	if err != nil {
		return err
	}

	instance := application.New(application.Options{
		Name:        applicationName,
		Description: applicationDescription,
		Services: []application.Service{
			application.NewService(featureServices.Collections),
			application.NewService(featureServices.Environments),
			application.NewService(featureServices.Git),
			application.NewService(featureServices.History),
			application.NewService(featureServices.HTTPClient),
			application.NewService(featureServices.Interop),
			application.NewService(featureServices.OAuth2),
			application.NewService(featureServices.Platform),
			application.NewService(featureServices.Secrets),
			application.NewService(featureServices.Updates),
			application.NewService(featureServices.Workspace),
			application.NewService(featureServices.Assistant),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendAssets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	if err := configureUpdates(instance, featureServices.Updates); err != nil {
		return err
	}

	windowOptions := application.WebviewWindowOptions{
		Title:            mainWindowTitle,
		Width:            mainWindowWidth,
		Height:           mainWindowHeight,
		MinWidth:         mainWindowMinWidth,
		BackgroundColour: application.NewRGB(30, 30, 30),
		Mac: application.MacWindow{
			// The web title bar declares its own drag region through the
			// --wails-draggable CSS property, so its buttons stay clickable.
			Backdrop: application.MacBackdropTranslucent,
			TitleBar: application.MacTitleBarHiddenInset,
		},
		URL: "/",
	}

	// Windows draws the web title bar itself. The native frame only follows
	// the operating system theme, set once at startup, while the application
	// theme is chosen in the window: a frameless window keeps its shadow,
	// rounded corners and resize borders, and the web header matches the
	// theme with its own minimize, maximize and close buttons.
	if runtime.GOOS == "windows" {
		windowOptions.Frameless = true
	}

	instance.Window.NewWithOptions(windowOptions)

	return instance.Run()
}
