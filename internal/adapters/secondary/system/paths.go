// Package system provides platform-specific filesystem helpers, such as
// locating the application's local data directory.
package system

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	appName    = "jobpilot"
	dbFileName = "jobpilot.db"
)

// GetDataDir returns the platform-specific application data directory.
func GetDataDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}

	return filepath.Join(configDir, appName), nil
}

// EnsureDataDir creates the application data directory if it does not already exist.
func EnsureDataDir() (string, error) {
	dir, err := GetDataDir()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create application data directory %s: %w", dir, err)
		}
	}

	return dir, nil
}

// DefaultDBPath returns the path to the application's SQLite database file,
// creating the data directory if needed. This is the single source of
// truth for that path: both cmd/jobpilot-cli and cmd/migrate call it, so
// the two can never disagree about which database file they're pointing at.
func DefaultDBPath() (string, error) {
	dir, err := EnsureDataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, dbFileName), nil
}
