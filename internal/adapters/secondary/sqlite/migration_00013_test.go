//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration00013_HackerNewsCompanyLinks repairs company names stored
// with their homepage inside them, and leaves every other name alone.
func TestMigration00013_HackerNewsCompanyLinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := NewDB(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	provider, err := db.migrationProvider()
	require.NoError(t, err)
	_, err = provider.UpTo(ctx, 12)
	require.NoError(t, err)

	names := map[string][2]string{
		"j1": {"hackernews", "Freeform ([http://freeform.co](http://freeform.co))"},
		"j2": {"hackernews", "Snout [https://snout.com/](https://snout.com/)"},
		"j3": {"hackernews", "Remote (US) Close ([https://close.com](https://close.com))"},
		"j4": {"hackernews", "Modash.io"},
		"j5": {"arbeitnow", "Weird [x](y) GmbH"},
	}
	for id, row := range names {
		_, err := db.Conn().ExecContext(ctx,
			`INSERT INTO jobs (id, canonical_url, source_id, title, company_name) VALUES (?, ?, ?, 'Engineer', ?)`,
			id, "https://example.com/"+id, row[0], row[1])
		require.NoError(t, err)
	}

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	want := map[string]string{
		"j1": "Freeform",
		"j2": "Snout",
		"j3": "Remote (US) Close",
		"j4": "Modash.io",
		"j5": "Weird [x](y) GmbH",
	}
	for id, name := range want {
		var got string
		require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT company_name FROM jobs WHERE id = ?`, id).Scan(&got))
		assert.Equal(t, name, got, id)
	}
}
