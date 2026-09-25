package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With no Chrome running, erasing the profile is removing its directory.
// Closing a running Chrome first is discardLocked, which every other path
// that ends a session already relies on.
func TestProfileEraser_RemovesTheProfile(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "browser-profile")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "Default"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Default", "Cookies"), []byte("session"), 0o600))

	eraser := NewChromeSession(dir, true).ProfileEraser()
	require.NoError(t, eraser.Erase(context.Background()))

	_, err := os.Stat(dir)
	assert.True(t, os.IsNotExist(err))
	assert.NoError(t, eraser.Erase(context.Background()), "erasing twice is not an error, so a failed delete-all can be repeated")
}
