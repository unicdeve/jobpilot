//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration00014_NormalizeTimestamps rewrites each form a time was
// stored in before it into UTC, and leaves NULLs alone.
func TestMigration00014_NormalizeTimestamps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := NewDB(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	provider, err := db.migrationProvider()
	require.NoError(t, err)
	_, err = provider.UpTo(ctx, 13)
	require.NoError(t, err)

	stored := map[string][2]any{
		// The driver's old default, with its monotonic reading.
		"j1": {"2026-09-15 19:36:10.572112 +0100 WAT m=+195.631690542", "2026-09-25 00:55:16 +0100 WAT"},
		// time.Time.String of a UTC value, and SQLite's CURRENT_TIMESTAMP.
		"j2": {"2026-09-12 01:29:24 +0000 UTC", "2026-09-12 01:29:24"},
		// Already normalized, and a NULL posting date.
		"j3": {"2026-09-20 08:00:00+00:00", nil},
	}
	for id, row := range stored {
		_, err := db.Conn().ExecContext(ctx,
			`INSERT INTO jobs (id, canonical_url, source_id, source_job_id, title, company_name, discovered_at, posted_at)
			VALUES (?, ?, 'arbeitnow', '', 'Engineer', 'Acme', ?, ?)`,
			id, "https://example.com/"+id, row[0], row[1])
		require.NoError(t, err)
	}

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	want := map[string][2]any{
		"j1": {"2026-09-15 18:36:10.572112+00:00", "2026-09-24 23:55:16+00:00"},
		"j2": {"2026-09-12 01:29:24+00:00", "2026-09-12 01:29:24+00:00"},
		"j3": {"2026-09-20 08:00:00+00:00", nil},
	}
	for id, row := range want {
		var discovered string
		var posted *string
		require.NoError(t, db.Conn().QueryRowContext(ctx,
			`SELECT CAST(discovered_at AS TEXT), CAST(posted_at AS TEXT) FROM jobs WHERE id = ?`, id).Scan(&discovered, &posted))
		assert.Equal(t, row[0], discovered, id)
		if row[1] == nil {
			assert.Nil(t, posted, id)
		} else {
			require.NotNil(t, posted, id)
			assert.Equal(t, row[1], *posted, id)
		}
	}

	// And the values read back as the same instants they were written as.
	var discovered time.Time
	require.NoError(t, db.Conn().QueryRowContext(ctx, `SELECT discovered_at FROM jobs WHERE id = 'j1'`).Scan(&discovered))
	assert.True(t, discovered.Equal(time.Date(2026, 9, 15, 18, 36, 10, 572112000, time.UTC)))
}

// TestMigration00014_RefusesUnreadableTime stops rather than guess at a
// value it cannot read.
func TestMigration00014_RefusesUnreadableTime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := NewDB(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	provider, err := db.migrationProvider()
	require.NoError(t, err)
	_, err = provider.UpTo(ctx, 13)
	require.NoError(t, err)

	_, err = db.Conn().ExecContext(ctx,
		`INSERT INTO jobs (id, canonical_url, source_id, title, company_name, discovered_at)
		VALUES ('j1', 'https://example.com/j1', 'arbeitnow', 'Engineer', 'Acme', 'last Tuesday')`)
	require.NoError(t, err)

	_, err = provider.Up(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jobs.discovered_at")
}

func TestParseLegacyTime(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]time.Time{
		"2026-09-15 19:36:10.572112 +0100 WAT m=+195.63": time.Date(2026, 9, 15, 18, 36, 10, 572112000, time.UTC),
		"2026-09-23 14:00:02 +0000 +0000":                time.Date(2026, 9, 23, 14, 0, 2, 0, time.UTC),
		"2026-09-12 01:29:24":                            time.Date(2026, 9, 12, 1, 29, 24, 0, time.UTC),
		"2026-09-12T01:29:24Z":                           time.Date(2026, 9, 12, 1, 29, 24, 0, time.UTC),
		"2026-09-12 02:29:24+01:00":                      time.Date(2026, 9, 12, 1, 29, 24, 0, time.UTC),
	} {
		got, err := parseLegacyTime(value)
		require.NoError(t, err, value)
		assert.True(t, got.Equal(want), "%s: got %s", value, got)
	}
}
