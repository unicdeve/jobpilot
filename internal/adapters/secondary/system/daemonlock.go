package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// daemonLockName is the file a running daemon holds locked.
const daemonLockName = "daemon.lock"

// ErrDaemonRunning is the lock held by another daemon's process.
var ErrDaemonRunning = errors.New("another jobpilot daemon is running")

// daemonLockPoll is how often a lock that is held is tried again.
const daemonLockPoll = 100 * time.Millisecond

// DaemonLock is an exclusive lock on a file in the data directory, which
// the daemon holds for as long as its process runs. The operating system
// lets go of it when the process exits, however it exits.
//
// It answers what the loopback API cannot: whether a daemon's process is
// still there. A stopping daemon stops answering requests before its
// Scout run has recorded how it ended and the database has closed, so a
// client replacing it waits for the lock instead (ADR-0029 §4). The
// desktop takes the same lock through Rust's File::try_lock, which uses
// the same flock and LockFileEx.
type DaemonLock struct {
	file *os.File
}

// LockDaemon takes the lock, waiting up to wait for another process to let
// go of it, and returns ErrDaemonRunning if it does not.
func LockDaemon(ctx context.Context, wait time.Duration) (*DaemonLock, error) {
	dir, err := EnsureDataDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, daemonLockName)
	deadline := time.Now().Add(wait)
	for {
		lock, err := tryLock(path)
		if !errors.Is(err, ErrDaemonRunning) || !time.Now().Before(deadline) {
			return lock, err
		}
		select {
		case <-ctx.Done():
			return nil, ErrDaemonRunning
		case <-time.After(daemonLockPoll):
		}
	}
}

// Release lets go of the lock.
func (l *DaemonLock) Release() error {
	return l.file.Close()
}

// DaemonRunning reports whether a daemon's process holds the lock: one
// running, starting, or still stopping.
func DaemonRunning() (bool, error) {
	dir, err := GetDataDir()
	if err != nil {
		return false, err
	}
	lock, err := tryLock(filepath.Join(dir, daemonLockName))
	switch {
	case err == nil:
		return false, lock.Release()
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	case errors.Is(err, ErrDaemonRunning):
		return true, nil
	default:
		return false, err
	}
}

// WaitForDaemonExit waits until no daemon's process holds the lock, or ctx
// ends.
func WaitForDaemonExit(ctx context.Context) error {
	for {
		running, err := DaemonRunning()
		if err != nil || !running {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(daemonLockPoll):
		}
	}
}

func tryLock(path string) (*DaemonLock, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &DaemonLock{file: file}, nil
}
