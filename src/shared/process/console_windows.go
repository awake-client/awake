//go:build windows

// Package process provides small helpers for starting child processes.
package process

import (
	"os/exec"
	"syscall"
)

// createNoWindow asks Windows to give the child process a console without a
// window. Awake runs as a graphical application, so a console program started
// without the flag opens a console window of its own, which flashes over the
// interface while the command runs.
const createNoWindow = 0x08000000

// HideConsoleWindow keeps a console program from opening a console window.
func HideConsoleWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
