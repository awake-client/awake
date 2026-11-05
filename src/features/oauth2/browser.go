package oauth2

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Browser opens an authorization page in the default browser.
type Browser interface {
	Open(rawURL string) error
}

type systemBrowser struct{}

func (systemBrowser) Open(rawURL string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", rawURL)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		command = exec.Command("xdg-open", rawURL)
	}

	if err := command.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
