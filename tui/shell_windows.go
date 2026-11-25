package tui

import (
	"os/exec"
	"syscall"
)

// shellCommand wraps the rendered SSH command in "cmd.exe /S /C", which finds
// the OpenSSH client that ships with Windows on PATH.
//
// The command line is set verbatim instead of going through Args, because Go
// escapes arguments with the MSVCRT rules while cmd.exe parses its own way; a
// template containing quotes would otherwise reach cmd.exe mangled. With /S,
// cmd.exe strips exactly the outer pair of quotes and runs the rest as typed.
func shellCommand(command string) *exec.Cmd {
	cmd := exec.Command("cmd.exe") //nolint:noctx // no parent context for the SSH hand-off
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /S /C "` + command + `"`}
	return cmd
}
