// Package updates checks for and installs application updates. The Wails
// updater downloads, verifies and swaps the binary without a window of its
// own; this package exposes each step to the frontend, which asks the user
// before installing and shows the changes of the new version.
package updates

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// Version is the running application version. Release builds override it with
// -ldflags "-X github.com/awake-client/awake/src/features/updates.Version=1.2.3";
// development builds keep "dev" so they never look like a published release.
var Version = "dev"

const (
	checkTimeout    = time.Minute
	installTimeout  = 15 * time.Minute
	restartTimeout  = time.Minute
	releaseTagKey   = "github.release.tag"
	releasePageKey  = "github.release.htmlURL"
	releaseTagStart = "v"
)

var (
	// ErrNotConfigured is returned when the update controller is missing.
	ErrNotConfigured = errors.New("updates: updater is not configured")
	// ErrNoUpdate is returned when installing without an available update.
	ErrNoUpdate = errors.New("updates: no update is available, check for updates first")
	// ErrBusy is returned when an update is already being installed.
	ErrBusy = errors.New("updates: an update is already being installed")
)

// Controller is the part of the Wails updater this feature drives.
type Controller interface {
	Check(ctx context.Context) (*updater.Release, error)
	DownloadAndInstall(ctx context.Context) error
	Restart(ctx context.Context) error
	State() updater.State
	CurrentVersion() string
}

// ChangelogSource reads the changelog published with a release tag.
type ChangelogSource interface {
	Changelog(ctx context.Context, tag string) (string, error)
}

// Release describes the update offered to the user.
type Release struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	// PublishedAt is an RFC 3339 date, empty when unknown.
	PublishedAt string `json:"publishedAt"`
	// URL is the release page, where the update can be downloaded by hand.
	URL string `json:"url"`
	// Size is the download size in bytes, 0 when unknown.
	Size int64 `json:"size"`
	// Changes lists every version between the running one and this release,
	// newest first.
	Changes []ChangelogVersion `json:"changes"`
}

// Status is the update state the frontend renders.
type Status struct {
	CurrentVersion string `json:"currentVersion"`
	// State is the updater lifecycle phase, such as "available" or "ready".
	State string `json:"state"`
	// Release is the update on offer, nil when the application is up to date.
	Release *Release `json:"release"`
	// InstallBlocker explains why the update must be installed by hand; empty
	// when the application can replace itself.
	InstallBlocker string `json:"installBlocker"`
}

// Service exposes update checks to the application layer.
type Service struct {
	mutex             sync.RWMutex
	controller        Controller
	changelog         ChangelogSource
	checkInstallation func() error
	release           *Release
}

// NewService returns an update service that stays idle until Use is called.
func NewService() *Service {
	return &Service{checkInstallation: checkInstallation}
}

// Use connects the service to the Wails updater and to the changelog of the
// releases. It is a package function so neither crosses the Wails bridge.
func Use(service *Service, controller Controller, changelog ChangelogSource) {
	service.mutex.Lock()
	service.controller = controller
	service.changelog = changelog
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

// Ready reports whether an update is installed and waits for a restart.
func (service *Service) Ready() bool {
	service.mutex.RLock()
	defer service.mutex.RUnlock()

	return service.controller != nil && service.controller.State() == updater.StateReady
}

// Status returns the current update state without contacting GitHub.
func (service *Service) Status() Status {
	service.mutex.RLock()
	controller := service.controller
	release := service.release
	service.mutex.RUnlock()

	if controller == nil {
		return Status{CurrentVersion: Version, State: string(updater.StateUnconfigured)}
	}
	return service.status(controller, release)
}

// Check asks GitHub for a newer release and returns it with its changes. It
// installs nothing: Install does, once the user has confirmed. While an update
// is being installed, it reports that update instead of checking again.
func (service *Service) Check() (Status, error) {
	service.mutex.RLock()
	controller := service.controller
	changelog := service.changelog
	release := service.release
	service.mutex.RUnlock()

	if controller == nil {
		return Status{}, ErrNotConfigured
	}
	if isInstalling(controller.State()) {
		return service.status(controller, release), nil
	}

	checkContext, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	found, err := controller.Check(checkContext)
	if err != nil {
		return Status{}, fmt.Errorf("check for updates: %w", err)
	}
	release = nil
	if found != nil {
		release = describeRelease(checkContext, found, controller.CurrentVersion(), changelog)
	}

	service.mutex.Lock()
	service.release = release
	service.mutex.Unlock()
	return service.status(controller, release), nil
}

// Install downloads, verifies and stages the update found by Check. It
// returns once the download has started; progress, completion and failures
// arrive as the updater events. The new version runs after Restart.
func (service *Service) Install() error {
	service.mutex.RLock()
	controller := service.controller
	checkInstallation := service.checkInstallation
	service.mutex.RUnlock()

	if controller == nil {
		return ErrNotConfigured
	}
	switch controller.State() {
	case updater.StateAvailable, updater.StateError:
	case updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
		return ErrBusy
	default:
		return ErrNoUpdate
	}
	if err := checkInstallation(); err != nil {
		return fmt.Errorf("install the update: %w", err)
	}

	go func() {
		installContext, cancel := context.WithTimeout(context.Background(), installTimeout)
		defer cancel()
		// The updater reports every failure of the flow as an error event,
		// which the frontend displays.
		_ = controller.DownloadAndInstall(installContext)
	}()
	return nil
}

// Restart quits the application and starts the installed update.
func (service *Service) Restart() error {
	service.mutex.RLock()
	controller := service.controller
	service.mutex.RUnlock()

	if controller == nil {
		return ErrNotConfigured
	}
	restartContext, cancel := context.WithTimeout(context.Background(), restartTimeout)
	defer cancel()
	if err := controller.Restart(restartContext); err != nil {
		return fmt.Errorf("restart into the update: %w", err)
	}
	return nil
}

func (service *Service) status(controller Controller, release *Release) Status {
	status := Status{
		CurrentVersion: controller.CurrentVersion(),
		State:          string(controller.State()),
		Release:        release,
	}
	if release != nil {
		if err := service.checkInstallation(); err != nil {
			status.InstallBlocker = err.Error()
		}
	}
	return status
}

func isInstalling(state updater.State) bool {
	switch state {
	case updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
		return true
	}
	return false
}

// describeRelease turns the release found by the updater into the one shown
// to the user. The changes come from the changelog of the release tag, which
// covers every version skipped since the running one; the release notes are
// the fallback when it cannot be read.
func describeRelease(
	operationContext context.Context,
	found *updater.Release,
	currentVersion string,
	changelog ChangelogSource,
) *Release {
	release := &Release{
		Version: found.Version,
		Name:    found.Name,
		URL:     metadataString(found.Metadata, releasePageKey),
		Size:    found.Artifact.Size,
	}
	if !found.PublishedAt.IsZero() {
		release.PublishedAt = found.PublishedAt.Format(time.RFC3339)
	}

	if changelog != nil {
		tag := metadataString(found.Metadata, releaseTagKey)
		if tag == "" {
			tag = releaseTagStart + found.Version
		}
		if content, err := changelog.Changelog(operationContext, tag); err == nil {
			release.Changes = changesBetween(parseChangelog(content), currentVersion, found.Version)
		}
	}
	if len(release.Changes) == 0 {
		release.Changes = parseReleaseNotes(found.Version, found.Notes)
	}
	return release
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}
