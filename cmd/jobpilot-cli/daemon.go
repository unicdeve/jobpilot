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
// is currently reachable, or replacing one older than this CLI. This
// implements the "CLI command starts a short-lived process when
// appropriate" lifecycle pattern from docs/01_architecture/01_daemon.md.
//
// What it has to say goes to stderr, so it never mixes with a command's
// output.
func ensureDaemon(ctx context.Context) (*httpapi.Client, error) {
	return httpapi.EnsureDaemon(ctx, findDaemonBinary, func(notice string) {
		fmt.Fprintln(os.Stderr, "jobpilot-cli: "+notice)
	})
}

// findDaemonBinary looks for the jobpilot daemon binary next to the running
// CLI binary first — the layout `make build` produces in bin/, and an
// installed app's — falling back to PATH.
//
// A CLI put on PATH as a link to the one inside the app is resolved to
// that one first, so it finds the daemon beside it.
func findDaemonBinary() (string, error) {
	name := "jobpilot"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
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
