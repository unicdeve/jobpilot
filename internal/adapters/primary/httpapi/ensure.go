package httpapi

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
)

const (
	// daemonStartTimeout bounds how long a client waits for a daemon it
	// started to become reachable.
	daemonStartTimeout  = 5 * time.Second
	daemonPollInterval  = 150 * time.Millisecond
	daemonHealthTimeout = 2 * time.Second
)

// EnsureDaemon connects to the running jobpilot daemon, starting the one at
// daemonPath if none answers. The CLI uses it, and so does `jobpilot scout
// wake`, which an operating system's scheduler runs (ADR-0027 §4): both
// are clients, and only ever one daemon serves the data directory.
func EnsureDaemon(ctx context.Context, daemonPath string) (*Client, error) {
	if client, ok := FindDaemon(ctx); ok {
		return client, nil
	}

	if err := spawnDaemon(daemonPath); err != nil {
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
	if err := client.Health(healthCtx); err != nil {
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
