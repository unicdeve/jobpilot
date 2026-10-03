package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// wakeTimeout bounds `scout wake` from start to finish. It only finds or
// starts the daemon and asks one question; the run itself, if there is
// one, belongs to the daemon and is not waited for.
const wakeTimeout = time.Minute

// wake is what a scheduled Scout's alarm runs: ask the daemon to run the
// Scout if it is due, starting the daemon if none is running, and exit
// (ADR-0027 §4). Nobody watches it run, so what it did goes to the
// daemon's log.
func wake(scoutID string) error {
	logger := wakeLogger()
	ctx, cancel := context.WithTimeout(context.Background(), wakeTimeout)
	defer cancel()

	self, err := executable()
	if err != nil {
		logger.Printf("scout wake %s: %v", scoutID, err)
		return err
	}
	client, err := httpapi.EnsureDaemon(ctx, self)
	if err != nil {
		logger.Printf("scout wake %s: %v", scoutID, err)
		return err
	}
	answer, err := client.WakeScout(ctx, scoutID)
	if err != nil {
		logger.Printf("scout wake %s: %v", scoutID, err)
		return err
	}

	switch {
	case answer.Outcome == domain.WakeNotDue && answer.NextRunAt != nil:
		logger.Printf("scout wake %s: not due until %s", scoutID, answer.NextRunAt.Local().Format(time.RFC3339))
	default:
		logger.Printf("scout wake %s: %s", scoutID, answer.Outcome)
	}
	return nil
}

// wakeLogger appends to the daemon's log, or writes to stderr if that
// cannot be opened.
func wakeLogger() *log.Logger {
	var out io.Writer = os.Stderr
	if dataDir, err := system.EnsureDataDir(); err == nil {
		file, err := os.OpenFile(filepath.Join(dataDir, system.DaemonLogName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			out = file
		}
	}
	return log.New(out, "", log.LstdFlags)
}

// executable is this program's own path, with links resolved, so an alarm
// names the file itself rather than a link that may later point elsewhere.
func executable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("finding the daemon's own executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path, nil
}

// alarmTarget is what every alarm starts: this executable, with the
// environment that makes it find this daemon's data directory.
//
// The data directory comes from os.UserConfigDir, which reads HOME on
// macOS, XDG_CONFIG_HOME (or HOME) on Linux and AppData on Windows. A job
// launchd or systemd starts does not get the user's shell environment, so
// the variable that decided this daemon's directory is carried into the
// alarm. Task Scheduler runs a task with the user's own profile, AppData
// included, and its tasks carry no environment.
func alarmTarget() (services.AlarmTarget, error) {
	self, err := executable()
	if err != nil {
		return services.AlarmTarget{}, err
	}
	env := map[string]string{}
	switch runtime.GOOS {
	case "darwin":
		if home := os.Getenv("HOME"); home != "" {
			env["HOME"] = home
		}
	case "linux":
		if config := os.Getenv("XDG_CONFIG_HOME"); config != "" {
			env["XDG_CONFIG_HOME"] = config
		}
	}
	return services.AlarmTarget{Executable: self, Env: env}, nil
}
