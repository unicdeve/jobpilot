package ports

import (
	"context"
	"errors"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// ErrAlarmsUnavailable is returned by a ScoutAlarms on a system that has no
// scheduler JobPilot can use, such as Linux without systemd. Scouts can
// still be run by hand there.
var ErrAlarmsUnavailable = errors.New("this system has no scheduler JobPilot can use")

// ScoutAlarms is the operating system's scheduler, holding one alarm per
// scheduled Scout: launchd on macOS, systemd on Linux, Task Scheduler on
// Windows (ADR-0027 §6, §7).
//
// It is dumb on purpose. The database is the schedule, and which alarms
// should exist is decided in core by comparing fingerprints; an adapter
// only lists, writes and removes.
type ScoutAlarms interface {
	// List returns every alarm JobPilot holds, recognised by its name.
	List(ctx context.Context) ([]domain.ScoutAlarmEntry, error)
	// Write creates the alarm, or replaces the one for the same Scout. An
	// alarm the user switched off outside JobPilot stays off.
	Write(ctx context.Context, alarm domain.ScoutAlarm) error
	// Remove deletes a Scout's alarm. Removing one that does not exist is
	// not an error.
	Remove(ctx context.Context, scoutID string) error
}
