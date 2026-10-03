//go:build darwin

package alarms

import (
	"os"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// New returns this system's scheduler: launchd.
func New() (ports.ScoutAlarms, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewLaunchd(home, os.Getuid()), nil
}
