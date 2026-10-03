//go:build windows

package alarms

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: schtasks is a console program, and
// the daemon that runs it has no console, so without it each call would
// open a window of its own.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
