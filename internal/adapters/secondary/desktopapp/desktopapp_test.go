package desktopapp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLauncher_StartsTheAppInItsDeliveryMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in app is a shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "args")
	app := filepath.Join(dir, "jobpilot-desktop")
	require.NoError(t, os.WriteFile(app, []byte("#!/bin/sh\necho \"$@\" > "+marker+"\n"), 0o755))

	require.NoError(t, Launcher{}.DeliverNotifications(context.Background(), app))

	assert.Eventually(t, func() bool {
		got, err := os.ReadFile(marker)
		return err == nil && string(got) == DeliverFlag+"\n"
	}, 5*time.Second, 10*time.Millisecond)
}

func TestLauncher_AnAppThatIsNotThereIsAnError(t *testing.T) {
	err := Launcher{}.DeliverNotifications(context.Background(), filepath.Join(t.TempDir(), "gone"))
	assert.ErrorContains(t, err, "not where it last said it was")
}
