//go:build windows

package alarms

import "github.com/unicdev/jobpilot/internal/core/ports"

// New returns this system's scheduler: Task Scheduler.
func New() (ports.ScoutAlarms, error) {
	return NewTaskScheduler(), nil
}
