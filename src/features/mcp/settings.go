package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/awake-client/awake/src/shared/files"
	"go.yaml.in/yaml/v3"
)

// settingsPath is the file holding the MCP settings of a workspace. It lives
// under .awake, which git ignores: each person decides for their computer.
var settingsPath = filepath.Join(".awake", "mcp.yaml")

// ErrDisabled is returned when the MCP server is turned off for a workspace.
var ErrDisabled = errors.New("the Awake MCP server is disabled for this workspace: turn it on in Awake, on the Assistant page")

type settings struct {
	// Disabled is stored rather than Enabled so a missing file means on.
	Disabled bool `yaml:"disabled"`
}

// Enabled reports whether agents may use the workspace. A workspace without
// settings is enabled.
func Enabled(workspaceRoot string) (bool, error) {
	content, err := os.ReadFile(filepath.Join(workspaceRoot, settingsPath))
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read MCP settings: %w", err)
	}
	var current settings
	if err := yaml.Unmarshal(content, &current); err != nil {
		return false, fmt.Errorf("read MCP settings: %w", err)
	}
	return !current.Disabled, nil
}

// SetEnabled turns the MCP server on or off for a workspace.
func SetEnabled(workspaceRoot string, enabled bool) error {
	path := filepath.Join(workspaceRoot, settingsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("write MCP settings: %w", err)
	}
	content, err := yaml.Marshal(settings{Disabled: !enabled})
	if err != nil {
		return fmt.Errorf("write MCP settings: %w", err)
	}
	return files.WriteAtomically(path, content)
}

// SettingsService lets the interface turn the MCP server on or off.
type SettingsService struct{}

// NewSettingsService returns the MCP settings service.
func NewSettingsService() *SettingsService {
	return &SettingsService{}
}

// Enabled reports whether agents may use the workspace through MCP.
func (*SettingsService) Enabled(workspaceRoot string) (bool, error) {
	return Enabled(workspaceRoot)
}

// SetEnabled turns the MCP server of a workspace on or off. A running server
// stops answering at its next call.
func (*SettingsService) SetEnabled(workspaceRoot string, enabled bool) error {
	return SetEnabled(workspaceRoot, enabled)
}
