package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
)

// ensureDaemon connects to a running jobpilot daemon, starting one if none
// is currently reachable. This implements the "CLI command starts a
// short-lived process when appropriate" lifecycle pattern from
// docs/01_architecture/01_daemon.md.
func ensureDaemon(ctx context.Context) (*httpapi.Client, error) {
	if client, ok := httpapi.FindDaemon(ctx); ok {
		return client, nil
	}
	daemonPath, err := findDaemonBinary()
	if err != nil {
		return nil, err
	}
	return httpapi.EnsureDaemon(ctx, daemonPath)
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
