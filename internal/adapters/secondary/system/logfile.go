package system

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DaemonLogName is the file the CLI and the desktop app send the daemon's
// output to, in the data directory.
const DaemonLogName = "daemon.log"

// DaemonLog is the daemon's log file, as something "delete all data"
// removes (ADR-0021 §7).
type DaemonLog struct {
	path string
}

// NewDaemonLog describes the daemon log in dataDir.
func NewDaemonLog(dataDir string) DaemonLog {
	return DaemonLog{path: filepath.Join(dataDir, DaemonLogName)}
}

// Describe names the log for a confirmation.
func (l DaemonLog) Describe() string { return "the daemon's log" }

// Erase empties the log. It is truncated rather than removed: the running
// daemon writes to it through a handle opened for appending, which keeps
// working, and writes after this land at the start of an empty file. A
// missing log is already erased.
func (l DaemonLog) Erase(context.Context) error {
	if err := os.Truncate(l.path, 0); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to empty the daemon's log: %w", err)
	}
	return nil
}
