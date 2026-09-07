//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB_InitializeSchema(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	wantTables := []string{"candidates", "resumes", "jobs", "job_matches", "user_settings"}
	for _, table := range wantTables {
		var name string
		err := db.Conn().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
		require.NoError(t, err, "expected table %q to exist after InitializeSchema", table)
		assert.Equal(t, table, name)
	}
}

func TestDB_InitializeSchema_Idempotent(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	// Calling it again (as every real startup does) must not error.
	assert.NoError(t, db.InitializeSchema(context.Background()))
}
