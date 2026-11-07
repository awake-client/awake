// Package updates checks for and installs application updates. The Wails
// updater owns the download window and the binary swap; this package exposes a
// small service to the frontend and keeps the controller injectable.
package updates

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// Version is the running application version. Release builds override it with
// -ldflags "-X github.com/clementherve/awake/src/features/updates.Version=1.2.3";
// development builds keep "dev" so they never look like a published release.
var Version = "dev"

const checkTimeout = 15 * time.Minute

var (
	// ErrNotConfigured is returned when the update controller is missing.
	ErrNotConfigured = errors.New("updates: updater is not configured")
)

// Controller is the part of the Wails updater this feature drives.
type Controller interface {
	CheckAndInstall(ctx context.Context) error
	State() updater.State
	CurrentVersion() string
}

// Service exposes update checks to the application layer.
type Service struct {
	mutex      sync.RWMutex
	controller Controller
}

// NewService returns an update service that stays idle until Use is called.
func NewService() *Service {
	return &Service{}
}

// Use connects the service to the Wails updater. It is a package function so
// the controller never crosses the Wails bridge.
func Use(service *Service, controller Controller) {
	service.mutex.Lock()
	service.controller = controller
	service.mutex.Unlock()
}

// CurrentVersion returns the running version.
func (service *Service) CurrentVersion() string {
	service.mutex.RLock()
	defer service.mutex.RUnlock()

	if service.controller == nil {
		return Version
	}
	return service.controller.CurrentVersion()
}

// State reports the updater lifecycle phase.
func (service *Service) State() string {
	service.mutex.RLock()
	defer service.mutex.RUnlock()

	if service.controller == nil {
		return string(updater.StateUnconfigured)
	}
	return string(service.controller.State())
}

// CheckForUpdates opens the update window and runs the update flow in the
// background. Progress and errors appear in that window.
func (service *Service) CheckForUpdates() error {
	service.mutex.RLock()
	controller := service.controller
	service.mutex.RUnlock()

	if controller == nil {
		return ErrNotConfigured
	}

	go func() {
		checkContext, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		_ = controller.CheckAndInstall(checkContext)
	}()

	return nil
}

// Ready reports whether an update is installed and waits for a restart.
func (service *Service) Ready() bool {
	service.mutex.RLock()
	defer service.mutex.RUnlock()

	return service.controller != nil && service.controller.State() == updater.StateReady
}
