package main

import (
	"context"
	"errors"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/alarms"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// removeAlarmsTimeout bounds `alarms remove`, which runs launchctl,
// systemctl or schtasks once per alarm.
const removeAlarmsTimeout = time.Minute

// removeAlarms removes every alarm JobPilot holds in the operating
// system's scheduler. The Windows uninstaller runs it after `jobpilot
// stop`, since nothing else would remove a Scout's task (ADR-0029 §5).
//
// A running daemon puts back the alarms of its scheduled Scouts the next
// time it reconciles them, so it refuses while one runs.
func removeAlarms() error {
	running, err := system.DaemonRunning()
	if err != nil {
		return err
	}
	if running {
		return errors.New("a jobpilot daemon is running, and would schedule its scouts again; stop it first with `jobpilot stop`")
	}
	scheduler, err := alarms.New()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), removeAlarmsTimeout)
	defer cancel()
	return services.RemoveAlarms(ctx, scheduler)
}
