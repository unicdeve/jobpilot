package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
)

const (
	// daemonStartTimeout bounds how long the CLI waits for a freshly
	// spawned daemon to become reachable before giving up.
	daemonStartTimeout  = 5 * time.Second
	daemonPollInterval  = 150 * time.Millisecond
	daemonHealthTimeout = 2 * time.Second
)

// ensureDaemon connects to a running jobpilot daemon, starting one if none
// is currently reachable. This implements the "CLI command starts a
// short-lived process when appropriate" lifecycle pattern from
// docs/01_architecture/01_daemon.md.
func ensureDaemon(ctx context.Context) (*httpapi.Client, error) {
	if client, ok := tryExistingDaemon(ctx); ok {
		return client, nil
	}

	if err := spawnDaemon(); err != nil {
		return nil, fmt.Errorf("starting jobpilot daemon: %w", err)
	}

	deadline := time.Now().Add(daemonStartTimeout)
	for time.Now().Before(deadline) {
		if client, ok := tryExistingDaemon(ctx); ok {
			return client, nil
		}
		time.Sleep(daemonPollInterval)
	}

	return nil, fmt.Errorf("jobpilot daemon did not become ready within %s", daemonStartTimeout)
}

// tryExistingDaemon checks the discovery file left by a running daemon and
// confirms it is actually reachable — a stale file can be left behind by an
// unclean shutdown, so the file alone is never trusted.
func tryExistingDaemon(ctx context.Context) (*httpapi.Client, bool) {
	info, err := httpapi.ReadDiscoveryInfo()
	if err != nil {
		return nil, false
	}

	client := httpapi.NewClient(info.Port)
	healthCtx, cancel := context.WithTimeout(ctx, daemonHealthTimeout)
	defer cancel()
	if err := client.Health(healthCtx); err != nil {
		return nil, false
	}
	return client, true
}

// spawnDaemon starts the jobpilot daemon binary as a detached background
// process, with its output sent to a log file rather than discarded (per
// docs/01_architecture/09_observability.md's "structured application
// logs") since nothing will be attached to its stdout/stderr afterward.
func spawnDaemon() error {
	daemonPath, err := findDaemonBinary()
	if err != nil {
		return err
	}

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

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching daemon binary %s: %w", daemonPath, err)
	}
	return nil
}

// findDaemonBinary looks for the jobpilot daemon binary next to the running
// CLI binary first — the layout `make build` produces in bin/ — falling
// back to PATH for installed setups.
func findDaemonBinary() (string, error) {
	name := "jobpilot"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("could not locate the %q daemon binary next to the running executable or on PATH", name)
}
