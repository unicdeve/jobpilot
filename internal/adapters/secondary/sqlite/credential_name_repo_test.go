//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func credRef(t *testing.T, source, label string) domain.CredentialRef {
	t.Helper()
	ref, err := domain.NewCredentialRef(source, label)
	require.NoError(t, err)
	return ref
}

func TestCredentialNameRepository(t *testing.T) {
	db := newTestDB(t)
	repo := NewCredentialNameRepository(db.Conn())
	ctx := context.Background()

	refs, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, refs)

	require.NoError(t, repo.Record(ctx, credRef(t, "adzuna", "work")))
	require.NoError(t, repo.Record(ctx, credRef(t, "adzuna", "personal")))
	require.NoError(t, repo.Record(ctx, credRef(t, "google-search", "default")))

	refs, err = repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, refs, 3)
	assert.Equal(t, "adzuna/personal", refs[0].String(), "ordered by source then label")
	assert.Equal(t, "adzuna/work", refs[1].String())
	assert.Equal(t, "google-search/default", refs[2].String())

	// Recording the same ref again refreshes it rather than duplicating:
	// setting a key a second time replaces it, and the directory must say
	// the same.
	require.NoError(t, repo.Record(ctx, credRef(t, "adzuna", "work")))
	refs, err = repo.List(ctx)
	require.NoError(t, err)
	assert.Len(t, refs, 3)

	require.NoError(t, repo.Forget(ctx, credRef(t, "adzuna", "work")))
	refs, err = repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	assert.Equal(t, "adzuna/personal", refs[0].String())

	// Forgetting one that was never recorded is not an error.
	assert.NoError(t, repo.Forget(ctx, credRef(t, "nowhere", "default")))
}

// TestCredentialNamesTableHoldsNoSecret is ADR-0019 §6 as a test. The table
// is a directory over the keychain; a column that could hold a value would
// put a secret in the application database, which AGENTS.md forbids.
func TestCredentialNamesTableHoldsNoSecret(t *testing.T) {
	db := newTestDB(t)

	rows, err := db.Conn().QueryContext(context.Background(), `PRAGMA table_info(credential_names)`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var (
			cid        int
			name, kind string
			notNull    int
			dflt       any
			pk         int
		)
		require.NoError(t, rows.Scan(&cid, &name, &kind, &notNull, &dflt, &pk))
		columns = append(columns, name)
	}
	require.NoError(t, rows.Err())

	assert.ElementsMatch(t, []string{"source", "label", "created_at", "updated_at"}, columns,
		"a new column here is how a secret would end up in the database")
}
