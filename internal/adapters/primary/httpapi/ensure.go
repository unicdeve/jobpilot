package httpapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/version"
)

const (
	// daemonStartTimeout bounds how long a client waits for a daemon it
	// started to become reachable.
	daemonStartTimeout  = 5 * time.Second
	daemonPollInterval  = 150 * time.Millisecond
	daemonHealthTimeout = 2 * time.Second
	// DaemonStopTimeout bounds waiting for a daemon to stop, or for one
	// that is starting or stopping to settle. Stopping cancels a scheduled
	// Scout run and waits for it to record how it ended, and for a
	// notification being delivered (ADR-0027 §4, ADR-0028): seconds as a
	// rule, and this is the limit past which something is wrong.
	DaemonStopTimeout = 2 * time.Minute
)

// EnsureDaemon connects to the running jobpilot daemon, starting one if
// none is running. The CLI uses it, and so does `jobpilot scout wake`,
// which an operating system's scheduler runs (ADR-0027 §4): both are
// clients, and only ever one daemon serves the data directory.
//
// A daemon older than this client is stopped and replaced; one newer is
// used, and notice is told this client is older (ADR-0029 §4). notice is
// also told when a daemon is being replaced, which waits for it to stop.
// daemonPath finds the daemon to start, and is asked only when one is.
func EnsureDaemon(ctx context.Context, daemonPath func() (string, error), notice func(string)) (*Client, error) {
	client, err := settledDaemon(ctx)
	if err != nil {
		return nil, err
	}
	if client != nil {
		daemonVersion, err := client.Health(ctx)
		if err != nil {
			return nil, fmt.Errorf("asking the jobpilot daemon its version: %w", err)
		}
		switch version.Compare(version.Version, daemonVersion) {
		case 0:
			return client, nil
		case -1:
			notice(fmt.Sprintf("this is JobPilot %s, which is older than the running daemon's %s", version.Version, daemonVersion))
			return client, nil
		}
		notice(fmt.Sprintf("replacing the running daemon, JobPilot %s, with %s", daemonVersion, version.Version))
		if err := StopDaemon(ctx); err != nil {
			return nil, fmt.Errorf("stopping the older jobpilot daemon: %w", err)
		}
	}

	path, err := daemonPath()
	if err != nil {
		return nil, err
	}
	if err := spawnDaemon(path); err != nil {
		return nil, fmt.Errorf("starting jobpilot daemon: %w", err)
	}

	deadline := time.Now().Add(daemonStartTimeout)
	for time.Now().Before(deadline) {
		if client, ok := FindDaemon(ctx); ok {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(daemonPollInterval):
		}
	}
	return nil, fmt.Errorf("jobpilot daemon did not become ready within %s", daemonStartTimeout)
}

// settledDaemon returns the daemon that answers, or nil when none is
// running. A daemon that holds the lock without answering is starting or
// stopping, and is waited for: starting another then would put two on one
// database.
func settledDaemon(ctx context.Context) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, DaemonStopTimeout)
	defer cancel()
	for {
		if client, ok := FindDaemon(ctx); ok {
			return client, nil
		}
		running, err := system.DaemonRunning()
		if err != nil {
			return nil, fmt.Errorf("checking for a running jobpilot daemon: %w", err)
		}
		if !running {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("a jobpilot daemon is running but did not answer within %s", DaemonStopTimeout)
		case <-time.After(daemonPollInterval):
		}
	}
}

// StopDaemon asks the running daemon to stop, and waits for its process to
// exit. With no daemon running, there is nothing to wait for.
func StopDaemon(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, DaemonStopTimeout)
	defer cancel()
	if client, ok := FindDaemon(ctx); ok {
		if err := client.Shutdown(ctx); err != nil {
			return fmt.Errorf("asking the jobpilot daemon to stop: %w", err)
		}
	}
	if err := system.WaitForDaemonExit(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("the jobpilot daemon did not stop within %s", DaemonStopTimeout)
		}
		return err
	}
	return nil
}

// FindDaemon returns a client for the daemon the discovery file names, if
// it answers. A file left by a daemon that did not shut down cleanly names
// one that does not, so the file alone is never trusted.
func FindDaemon(ctx context.Context) (*Client, bool) {
	info, err := ReadDiscoveryInfo()
	if err != nil {
		return nil, false
	}
	client := NewClient(info.Port)
	healthCtx, cancel := context.WithTimeout(ctx, daemonHealthTimeout)
	defer cancel()
	if _, err := client.Health(healthCtx); err != nil {
		return nil, false
	}
	return client, true
}

// spawnDaemon starts the daemon as a detached background process, with its
// output sent to the daemon's log file, since nothing will be attached to
// it afterwards (docs/01_architecture/09_observability.md).
//
// Detached means in a session of its own, or with no console on Windows,
// so that it outlives the process that started it. That matters most for
// `scout wake`: launchd and systemd stop what a finished job leaves behind
// in its process group or cgroup, and the units JobPilot writes ask them
// not to, but a daemon in its own session does not depend on being asked.
func spawnDaemon(daemonPath string) error {
	dataDir, err := system.EnsureDataDir()
	if err != nil {
		return fmt.Errorf("resolving data directory for daemon log: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, system.DaemonLogName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening daemon log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(daemonPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching daemon binary %s: %w", daemonPath, err)
	}
	// Nothing waits on it; releasing it lets this process exit without
	// leaving a zombie behind on the way.
	return cmd.Process.Release()
}
