// Package files provides small file system helpers shared by features.
package files

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomically writes content to path through a temporary file in the same
// directory, so readers never observe a partially written file.
func WriteAtomically(path string, content []byte) error {
	directory := filepath.Dir(path)

	temporaryFile, err := os.CreateTemp(directory, ".awake-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporaryFile.Name()

	defer func() {
		_ = os.Remove(temporaryPath)
	}()

	if _, err := temporaryFile.Write(content); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return fmt.Errorf("set permissions on %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}

	return nil
}
