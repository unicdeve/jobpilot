// Package desktopapp starts the desktop app on the daemon's behalf: in its
// delivery mode, to post notifications under JobPilot's own name
// (ADR-0028 §1).
package desktopapp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// DeliverFlag is what the desktop app is started with to post the
// notifications waiting and exit, opening no window.
const DeliverFlag = "--deliver-notifications"

// deliveryLimit bounds a delivery process. Posting a handful of
// notifications takes well under a second; one still running after this
// is stuck, and is stopped rather than left behind.
const deliveryLimit = time.Minute

// Launcher starts the desktop app's executable.
type Launcher struct{}

var _ ports.DesktopApp = Launcher{}

// DeliverNotifications starts the app at executable in its delivery mode
// and returns once it has started. The process is waited on in the
// background, so it leaves nothing behind, and stopped if it outlasts
// deliveryLimit.
func (Launcher) DeliverNotifications(_ context.Context, executable string) error {
	if _, err := os.Stat(executable); err != nil {
		return fmt.Errorf("the desktop app is not where it last said it was: %w", err)
	}
	cmd := exec.Command(executable, DeliverFlag)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the desktop app to post notifications: %w", err)
	}
	go func() {
		timer := time.AfterFunc(deliveryLimit, func() { _ = cmd.Process.Kill() })
		defer timer.Stop()
		if err := cmd.Wait(); err != nil {
			slog.Warn("the desktop app did not post notifications cleanly", "error", err)
		}
	}()
	return nil
}
