package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonLog_Erase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, DaemonLogName)

	// An appending writer, as the CLI and desktop hold one, stays usable.
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	_, err = writer.WriteString("candidate: ada@example.com\n")
	require.NoError(t, err)

	log := NewDaemonLog(dir)
	require.NoError(t, log.Erase(context.Background()))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Empty(t, content)

	_, err = writer.WriteString("after\n")
	require.NoError(t, err)
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "after\n", string(content), "later output starts an empty file rather than leaving a gap")

	assert.NoError(t, NewDaemonLog(t.TempDir()).Erase(context.Background()), "a log that was never written is already erased")
}
