package updates

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

type fakeController struct {
	called  chan struct{}
	state   updater.State
	version string
	err     error
}

func (controller *fakeController) CheckAndInstall(context.Context) error {
	if controller.called != nil {
		controller.called <- struct{}{}
	}
	return controller.err
}

func (controller *fakeController) State() updater.State {
	return controller.state
}

func (controller *fakeController) CurrentVersion() string {
	return controller.version
}

func TestCheckForUpdatesRequiresController(t *testing.T) {
	err := NewService().CheckForUpdates()
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestCheckForUpdatesRunsInTheBackground(t *testing.T) {
	called := make(chan struct{}, 1)
	service := NewService()
	Use(service, &fakeController{called: called})

	if err := service.CheckForUpdates(); err != nil {
		t.Fatalf("check for updates: %v", err)
	}

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("expected the update check to run")
	}
}

func TestServiceReportsControllerState(t *testing.T) {
	service := NewService()
	Use(service, &fakeController{
		state:   updater.StateReady,
		version: "1.2.3",
	})

	if service.CurrentVersion() != "1.2.3" {
		t.Errorf("unexpected version: %q", service.CurrentVersion())
	}
	if service.State() != string(updater.StateReady) {
		t.Errorf("unexpected state: %q", service.State())
	}
	if !service.Ready() {
		t.Error("expected the service to report a ready update")
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
}
