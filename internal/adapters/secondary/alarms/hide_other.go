//go:build !windows

package alarms

import "os/exec"

// hideWindow does nothing where a program run from the daemon opens no
// window.
func hideWindow(*exec.Cmd) {}
