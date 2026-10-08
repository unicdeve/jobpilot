package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/alarms"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/sqlite"
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

// restoreAlarms schedules every enabled, scheduled Scout again, as a
// starting daemon does, and exits. The Windows installer runs it once the
// new files are in place: upgrading runs the old version's uninstaller,
// whose `alarms remove` cannot tell an upgrade from an uninstall, and
// nothing else would schedule the Scouts again until JobPilot was next
// opened (ADR-0029 §5, as amended).
//
// With no database there is nothing to restore, and nothing is created. A
// running daemon keeps the alarms itself. Otherwise it holds the daemon's
// lock while it works, so a daemon cannot start on the database under it.
func restoreAlarms() error {
	catalog, err := system.OpenWorkspaceStore()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), removeAlarmsTimeout)
	defer cancel()
	lock, err := system.LockDaemon(ctx, daemonLockWait)
	if errors.Is(err, system.ErrDaemonRunning) {
		log.Print("jobpilot alarms restore: a daemon is running, and keeps the alarms itself")
		return nil
	}
	if err != nil {
		return fmt.Errorf("locking the data directory: %w", err)
	}
	defer func() { _ = lock.Release() }()

	workspaces, _ := catalog.List()
	for _, workspace := range workspaces {
		dbPath := catalog.DBPath(workspace)
		if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		db, err := sqlite.NewDB(dbPath)
		if err != nil {
			return fmt.Errorf("opening workspace %s database: %w", workspace.ID, err)
		}
		if err := db.InitializeSchema(ctx); err != nil {
			_ = db.Close()
			return fmt.Errorf("initializing workspace %s database schema: %w", workspace.ID, err)
		}
		namespace := workspace.ID
		if workspace.Legacy {
			namespace = ""
		}
		svc, err := wireServices(db, catalog.Dir(workspace), namespace)
		if err == nil {
			err = svc.Scouts.Reconcile(ctx)
		}
		closeErr := db.Close()
		if err != nil {
			return fmt.Errorf("restoring workspace %s alarms: %w", workspace.ID, err)
		}
		if closeErr != nil {
			return fmt.Errorf("closing workspace %s database: %w", workspace.ID, closeErr)
		}
	}
	return nil
}
