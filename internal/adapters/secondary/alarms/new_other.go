//go:build !darwin && !linux && !windows

package alarms

import (
	"context"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// New returns a scheduler that says there is none: JobPilot schedules
// Scouts on macOS, Linux and Windows only.
func New() (ports.ScoutAlarms, error) {
	return unavailable{}, nil
}

type unavailable struct{}

func (unavailable) List(context.Context) ([]domain.ScoutAlarmEntry, error) {
	return nil, ports.ErrAlarmsUnavailable
}

func (unavailable) Write(context.Context, domain.ScoutAlarm) error { return ports.ErrAlarmsUnavailable }

func (unavailable) Remove(context.Context, string) error { return nil }
