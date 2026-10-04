package system

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useDataDir points the application data directory at a temporary one, on
// every platform os.UserConfigDir reads.
func useDataDir(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("AppData", home)
}

func TestDaemonLock_IsHeldByOneAtATime(t *testing.T) {
	useDataDir(t)
	ctx := context.Background()

	lock, err := LockDaemon(ctx, 0)
	require.NoError(t, err)

	_, err = LockDaemon(ctx, 0)
	assert.ErrorIs(t, err, ErrDaemonRunning)

	require.NoError(t, lock.Release())
	again, err := LockDaemon(ctx, 0)
	require.NoError(t, err)
	require.NoError(t, again.Release())
}

func TestDaemonLock_WaitsForTheHolderToLetGo(t *testing.T) {
	useDataDir(t)
	ctx := context.Background()
	lock, err := LockDaemon(ctx, 0)
	require.NoError(t, err)
	time.AfterFunc(300*time.Millisecond, func() { _ = lock.Release() })

	again, err := LockDaemon(ctx, 5*time.Second)

	require.NoError(t, err)
	require.NoError(t, again.Release())
}

func TestWaitForDaemonExit(t *testing.T) {
	useDataDir(t)
	ctx := context.Background()
	require.NoError(t, WaitForDaemonExit(ctx), "no daemon has run")

	lock, err := LockDaemon(ctx, 0)
	require.NoError(t, err)
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, WaitForDaemonExit(short), context.DeadlineExceeded)

	time.AfterFunc(200*time.Millisecond, func() { _ = lock.Release() })
	assert.NoError(t, WaitForDaemonExit(ctx))
}

// TestDaemonLock_IsLetGoWhenTheProcessDies holds the lock in another
// process and kills it, as a daemon that crashed would be: the operating
// system lets go of the lock, and nothing is left to clean up.
func TestDaemonLock_IsLetGoWhenTheProcessDies(t *testing.T) {
	if os.Getenv("JOBPILOT_HOLD_DAEMON_LOCK") != "" {
		holdDaemonLock()
		return
	}
	useDataDir(t)
	ctx := context.Background()

	holder := exec.Command(os.Args[0], "-test.run=^TestDaemonLock_IsLetGoWhenTheProcessDies$")
	holder.Env = append(os.Environ(), "JOBPILOT_HOLD_DAEMON_LOCK=1")
	out, err := holder.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, holder.Start())
	t.Cleanup(func() { _ = holder.Process.Kill(); _ = holder.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "locked\n", line)

	_, err = LockDaemon(ctx, 0)
	assert.ErrorIs(t, err, ErrDaemonRunning, "held by the other process")

	require.NoError(t, holder.Process.Kill())
	_ = holder.Wait()
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	assert.NoError(t, WaitForDaemonExit(waitCtx))
}

func holdDaemonLock() {
	if _, err := LockDaemon(context.Background(), 0); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("locked")
	time.Sleep(time.Minute)
	os.Exit(0)
}
