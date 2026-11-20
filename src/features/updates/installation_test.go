package updates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallationTargetIsTheMacOSBundle(t *testing.T) {
	executable := filepath.FromSlash("/Applications/Awake.app/Contents/MacOS/awake")

	if got := installationTarget(executable, "darwin"); got != filepath.FromSlash("/Applications/Awake.app") {
		t.Errorf("got %q, want the bundle", got)
	}
	if got := installationTarget(executable, "linux"); got != executable {
		t.Errorf("got %q, want the executable", got)
	}
}

func TestProbeWritable(t *testing.T) {
	directory := t.TempDir()
	if err := probeWritable(directory); err != nil {
		t.Errorf("expected %s to be writable: %v", directory, err)
	}
	if err := probeWritable(filepath.Join(directory, "missing")); err == nil {
		t.Error("expected a missing directory to be reported")
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 0 {
		t.Errorf("the probe left %d files behind", len(entries))
	}
}
