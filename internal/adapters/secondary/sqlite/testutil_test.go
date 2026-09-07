//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestDB opens a fresh temp-file SQLite database with schema migrations
// applied, mirroring real startup (cmd/jobpilot-cli/main.go's dbPath ->
// NewDB -> InitializeSchema sequence). The database and connection are
// cleaned up automatically when the test ends.
func newTestDB(t *testing.T) *DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	require.NoError(t, db.InitializeSchema(context.Background()))
	return db
}
