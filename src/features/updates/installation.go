package updates

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/awake-client/awake/src/shared/semver"
)

// installationTarget returns what an update replaces: the .app bundle on
// macOS, the executable itself elsewhere. It mirrors what the Wails updater
// swaps so the permission check looks at the right directory.
func installationTarget(executable, platform string) string {
	if platform != "darwin" {
		return executable
	}
	parts := strings.Split(filepath.Clean(executable), string(filepath.Separator))
	for position, part := range parts {
		if strings.HasSuffix(part, ".app") {
			return string(filepath.Separator) + filepath.Join(parts[1:position+1]...)
		}
	}
	return executable
}

// checkInstallation explains why this copy of the application cannot replace
// itself, or returns nil when it can. The updater renames the new version over
// the old one, so it needs to write to the directory that holds it: an
// installation in Program Files, /usr/local/bin or an /Applications folder
// owned by another account cannot be updated in place.
func checkInstallation() error {
	if !semver.IsRelease(Version) {
		return fmt.Errorf("this development build cannot update itself")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the application: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	directory := filepath.Dir(installationTarget(executable, runtime.GOOS))
	if err := probeWritable(directory); err != nil {
		return fmt.Errorf("cannot write to %s, where Awake is installed", directory)
	}
	return nil
}

// probeWritable creates and removes a file in directory, which is the only
// check that holds on every platform, ACLs and read-only mounts included.
func probeWritable(directory string) error {
	probe, err := os.CreateTemp(directory, ".awake-update-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
