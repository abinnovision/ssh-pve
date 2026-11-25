//go:build !windows

package tui

import "os/exec"

// shellCommand wraps the rendered SSH command in "sh -c".
func shellCommand(command string) *exec.Cmd {
	return exec.Command("sh", "-c", command) //nolint:gosec,noctx // command is user-controlled via config; shell interpretation is intentional; no parent context for the SSH hand-off
}
