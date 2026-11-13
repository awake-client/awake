// Package platform exposes host operating system information to the frontend
// so the interface can adapt its chrome without platform-specific builds.
package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Service answers questions about the host platform.
type Service struct{}

// NewService returns a platform service.
func NewService() *Service {
	return &Service{}
}

// Name returns the operating system identifier used by Go: darwin, linux or
// windows.
func (service *Service) Name() string {
	return runtime.GOOS
}

// MCPCommand returns the command registering this Awake executable as the MCP
// server of a workspace in Claude Code, ready to paste in a terminal.
func (service *Service) MCPCommand(workspaceRoot string, readOnly bool) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return mcpCommand(executable, workspaceRoot, readOnly), nil
}

func mcpCommand(executable string, workspaceRoot string, readOnly bool) string {
	parts := []string{"claude", "mcp", "add", "awake", "--", shellQuote(executable), "mcp", "--workspace", shellQuote(workspaceRoot)}
	if readOnly {
		parts = append(parts, "--read-only")
	}
	return strings.Join(parts, " ")
}

// shellQuote quotes a value for POSIX shells when it needs it.
func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(character rune) bool {
		return !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("/._-+:@", character))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
