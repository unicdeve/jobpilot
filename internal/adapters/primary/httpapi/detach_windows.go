//go:build windows

package httpapi

import (
	"os/exec"
	"syscall"
)

// Process creation flags: a process group of its own, so a console's
// Ctrl+C does not reach it, and no console, so no window opens for it.
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// detach starts the daemon with no console and in a group of its own.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}
