package httpapi

import (
	"os"
	"testing"

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

func TestRemoveDiscoveryInfo(t *testing.T) {
	t.Run("removes the file naming this daemon", func(t *testing.T) {
		useDataDir(t)
		require.NoError(t, WriteDiscoveryInfo(DiscoveryInfo{PID: 100, Port: 5000}))

		require.NoError(t, RemoveDiscoveryInfo(100))

		_, err := ReadDiscoveryInfo()
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("leaves the file of a daemon started since", func(t *testing.T) {
		useDataDir(t)
		require.NoError(t, WriteDiscoveryInfo(DiscoveryInfo{PID: 200, Port: 6000}))

		require.NoError(t, RemoveDiscoveryInfo(100))

		info, err := ReadDiscoveryInfo()
		require.NoError(t, err)
		assert.Equal(t, DiscoveryInfo{PID: 200, Port: 6000}, *info)
	})

	t.Run("no file is not an error", func(t *testing.T) {
		useDataDir(t)
		assert.NoError(t, RemoveDiscoveryInfo(100))
	})
}
