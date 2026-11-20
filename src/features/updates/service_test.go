package updates

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

type fakeController struct {
	mutex     sync.Mutex
	state     updater.State
	version   string
	release   *updater.Release
	checkErr  error
	checks    int
	installed chan struct{}
	restarted bool
}

func (controller *fakeController) Check(context.Context) (*updater.Release, error) {
	controller.mutex.Lock()
	defer controller.mutex.Unlock()
	controller.checks++
	if controller.checkErr != nil {
		controller.state = updater.StateError
		return nil, controller.checkErr
	}
	if controller.release == nil {
		controller.state = updater.StateUpToDate
		return nil, nil
	}
	controller.state = updater.StateAvailable
	return controller.release, nil
}

func (controller *fakeController) DownloadAndInstall(context.Context) error {
	controller.mutex.Lock()
	controller.state = updater.StateReady
	controller.mutex.Unlock()
	if controller.installed != nil {
		controller.installed <- struct{}{}
	}
	return nil
}

func (controller *fakeController) Restart(context.Context) error {
	controller.mutex.Lock()
	defer controller.mutex.Unlock()
	controller.restarted = true
	return nil
}

func (controller *fakeController) State() updater.State {
	controller.mutex.Lock()
	defer controller.mutex.Unlock()
	return controller.state
}

func (controller *fakeController) CurrentVersion() string {
	return controller.version
}

type fakeChangelog struct {
	content string
	err     error
	tag     string
}

func (changelog *fakeChangelog) Changelog(_ context.Context, tag string) (string, error) {
	changelog.tag = tag
	return changelog.content, changelog.err
}

const sampleChangelog = `# Changelog

## [Unreleased]

### Added

- Something not shipped yet.

## [0.9.0] - 2026-11-20

### Added

- Request scripts store collection
  variables.

### Fixed

- No more console window on Windows.

## [0.8.1] - 2026-11-14

### Fixed

- A crash.

## [0.8.0] - 2026-11-10

### Added

- Copy as curl.

[0.9.0]: https://github.com/awake-client/awake/compare/v0.8.1...v0.9.0
`

func installableService(controller Controller, changelog ChangelogSource) *Service {
	service := NewService()
	service.checkInstallation = func() error { return nil }
	Use(service, controller, changelog)
	return service
}

func availableRelease() *updater.Release {
	return &updater.Release{
		Version:     "0.9.0",
		Name:        "Awake 0.9.0",
		Notes:       "## What's Changed\n* Generated note",
		PublishedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
		Artifact:    updater.Artifact{Size: 1234},
		Metadata: map[string]any{
			releaseTagKey:  "v0.9.0",
			releasePageKey: "https://github.com/awake-client/awake/releases/tag/v0.9.0",
		},
	}
}

func TestCheckRequiresController(t *testing.T) {
	if _, err := NewService().Check(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestCheckReportsUpToDate(t *testing.T) {
	service := installableService(&fakeController{version: "0.9.0"}, nil)

	status, err := service.Check()
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if status.Release != nil || status.State != string(updater.StateUpToDate) {
		t.Errorf("expected up to date, got %+v", status)
	}
}

func TestCheckListsEveryVersionSinceTheRunningOne(t *testing.T) {
	changelog := &fakeChangelog{content: sampleChangelog}
	service := installableService(&fakeController{version: "0.8.0", release: availableRelease()}, changelog)

	status, err := service.Check()
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if changelog.tag != "v0.9.0" {
		t.Errorf("read the changelog of %q, want v0.9.0", changelog.tag)
	}
	release := status.Release
	if release == nil {
		t.Fatal("expected a release")
	}
	if release.URL == "" || release.Size != 1234 || release.PublishedAt != "2026-11-20T10:00:00Z" {
		t.Errorf("unexpected release details: %+v", release)
	}
	var versions []string
	for _, change := range release.Changes {
		versions = append(versions, change.Version)
	}
	if strings.Join(versions, ",") != "0.9.0,0.8.1" {
		t.Errorf("got versions %v, want 0.9.0 and 0.8.1", versions)
	}
	if got := release.Changes[0].Sections[0].Items[0]; got != "Request scripts store collection variables." {
		t.Errorf("wrapped item not joined: %q", got)
	}
}

func TestCheckFallsBackToReleaseNotes(t *testing.T) {
	changelog := &fakeChangelog{err: errors.New("offline")}
	service := installableService(&fakeController{version: "0.8.0", release: availableRelease()}, changelog)

	status, err := service.Check()
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	changes := status.Release.Changes
	if len(changes) != 1 || changes[0].Sections[0].Title != "What's Changed" ||
		changes[0].Sections[0].Items[0] != "Generated note" {
		t.Errorf("unexpected fallback changes: %+v", changes)
	}
}

func TestCheckReportsWhyTheUpdateMustBeInstalledByHand(t *testing.T) {
	service := installableService(&fakeController{version: "0.8.0", release: availableRelease()}, nil)
	service.checkInstallation = func() error { return errors.New("read-only") }

	status, err := service.Check()
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if status.InstallBlocker != "read-only" {
		t.Errorf("got blocker %q", status.InstallBlocker)
	}
	if err := service.Install(); err == nil {
		t.Error("expected Install to refuse")
	}
}

func TestCheckDoesNotInterruptAnInstallation(t *testing.T) {
	controller := &fakeController{version: "0.8.0", state: updater.StateDownloading}
	service := installableService(controller, nil)

	status, err := service.Check()
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if controller.checks != 0 || status.State != string(updater.StateDownloading) {
		t.Errorf("checked during a download: %+v", status)
	}
}

func TestInstallRequiresAnAvailableUpdate(t *testing.T) {
	service := installableService(&fakeController{state: updater.StateIdle}, nil)

	if err := service.Install(); !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("expected ErrNoUpdate, got %v", err)
	}
}

func TestInstallRunsInTheBackgroundThenRestarts(t *testing.T) {
	installed := make(chan struct{}, 1)
	controller := &fakeController{version: "0.8.0", release: availableRelease(), installed: installed}
	service := installableService(controller, nil)

	if _, err := service.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if err := service.Install(); err != nil {
		t.Fatalf("install: %v", err)
	}
	select {
	case <-installed:
	case <-time.After(time.Second):
		t.Fatal("expected the update to install")
	}
	if !service.Ready() {
		t.Error("expected the update to be ready")
	}
	if err := service.Install(); !errors.Is(err, ErrBusy) {
		t.Errorf("expected ErrBusy once ready, got %v", err)
	}
	if err := service.Restart(); err != nil || !controller.restarted {
		t.Errorf("expected a restart, got %v", err)
	}
}

func TestServiceDefaultsBeforeConfiguration(t *testing.T) {
	service := NewService()

	if service.CurrentVersion() != Version {
		t.Errorf("unexpected default version: %q", service.CurrentVersion())
	}
	if service.State() != string(updater.StateUnconfigured) {
		t.Errorf("unexpected default state: %q", service.State())
	}
	if service.Ready() {
		t.Error("expected the service not to report a ready update")
	}
	if !errors.Is(service.Install(), ErrNotConfigured) || !errors.Is(service.Restart(), ErrNotConfigured) {
		t.Error("expected install and restart to require a controller")
	}
}
