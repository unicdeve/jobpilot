//go:build !windows

package httpapi

import (
	"os/exec"
	"syscall"
)

// detach starts the daemon in a session of its own.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
