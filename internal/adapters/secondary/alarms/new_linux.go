//go:build linux

package alarms

import (
	"os"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// New returns this system's scheduler: systemd's user manager. Where there
// is none, its List says so and scheduling is unavailable.
func New() (ports.ScoutAlarms, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewSystemd(configDir), nil
}
