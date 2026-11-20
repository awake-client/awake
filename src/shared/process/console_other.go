//go:build !windows

// Package process provides small helpers for starting child processes.
package process

import "os/exec"

// HideConsoleWindow does nothing where a child process gets no window of its
// own.
func HideConsoleWindow(command *exec.Cmd) {}
