package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
)

// discoveryFileName is the well-known file a running daemon writes so local
// clients (the CLI, eventually the desktop app) can find it without a fixed,
// pre-agreed port. This is the same discovery pattern tools like Ollama use
// for their own loopback HTTP APIs.
const discoveryFileName = "daemon.json"

// DiscoveryInfo describes a running daemon: which process it is and which
// loopback port it is listening on.
type DiscoveryInfo struct {
	PID  int `json:"pid"`
	Port int `json:"port"`
}

// discoveryPath returns the path to the daemon discovery file inside the
// application data directory.
func discoveryPath() (string, error) {
	dir, err := system.EnsureDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, discoveryFileName), nil
}

// WriteDiscoveryInfo persists info so other local processes can find the
// running daemon. Callers should remove it (RemoveDiscoveryInfo) on clean
// shutdown.
func WriteDiscoveryInfo(info DiscoveryInfo) error {
	path, err := discoveryPath()
	if err != nil {
		return fmt.Errorf("failed to resolve discovery file path: %w", err)
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to encode discovery info: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write discovery file %s: %w", path, err)
	}
	return nil
}

// ReadDiscoveryInfo reads the discovery file left by a running daemon. It
// returns an error if no daemon has ever started (or its discovery file was
// removed on shutdown) — the caller cannot distinguish "no daemon" from
// "daemon exited uncleanly, stale file removed" from this alone, so it
// should always follow up with a Client.Health check before trusting it.
func ReadDiscoveryInfo() (*DiscoveryInfo, error) {
	path, err := discoveryPath()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve discovery file path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery file %s: %w", path, err)
	}

	var info DiscoveryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse discovery file %s: %w", path, err)
	}
	return &info, nil
}

// RemoveDiscoveryInfo deletes the discovery file if it still names the
// daemon with the given PID. A daemon that is shutting down must not remove
// the file of one that has started since. It is not an error if the file
// is already gone.
func RemoveDiscoveryInfo(pid int) error {
	info, err := ReadDiscoveryInfo()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.PID != pid {
		return nil
	}

	path, err := discoveryPath()
	if err != nil {
		return fmt.Errorf("failed to resolve discovery file path: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove discovery file %s: %w", path, err)
	}
	return nil
}
