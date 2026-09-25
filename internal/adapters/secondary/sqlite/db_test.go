//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
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

func TestDB_JournalModeIsWAL(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	var mode string
	require.NoError(t, db.Conn().QueryRow("PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", strings.ToLower(mode))
}

// TestDB_ForeignKeysEnforcedOnEveryConnection guards the pool-wide pragma
// contract. Enabling foreign keys with a single db.Exec applies it to
// whichever pooled connection that call happened to take, so the enforcement
// a schema's FOREIGN KEY clauses promise held only by luck.
func TestDB_ForeignKeysEnforcedOnEveryConnection(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	// Hold several connections open simultaneously so the pool is forced to
	// create more than one, then check the constraint on each of them.
	const connections = 8
	held := make([]*sql.Conn, 0, connections)
	for i := 0; i < connections; i++ {
		c, err := db.Conn().Conn(ctx)
		require.NoError(t, err)
		held = append(held, c)
	}

	for i, c := range held {
		var enabled int
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled))
		assert.Equal(t, 1, enabled, "foreign_keys off on pooled connection %d", i)

		var secureDelete int
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA secure_delete").Scan(&secureDelete))
		assert.Equal(t, 1, secureDelete, "secure_delete off on pooled connection %d", i)

		_, err := c.ExecContext(ctx,
			`INSERT INTO resumes (id, candidate_id, name) VALUES (?, ?, ?)`,
			"resume-"+strconv.Itoa(i), "no-such-candidate", "CV",
		)
		assert.ErrorContains(t, err, "FOREIGN KEY constraint failed",
			"pooled connection %d did not enforce foreign keys", i)

		require.NoError(t, c.Close())
	}
}

// TestDB_ConcurrentReadWriteDoesNotReportBusy reproduces the desktop app's
// startup failure: several API requests arrive at once, one of them saving
// newly discovered jobs while the others read, and a reader fails outright
// with "database is locked (5) (SQLITE_BUSY)" instead of waiting. WAL plus a
// busy timeout is what makes the reads wait.
func TestDB_ConcurrentReadWriteDoesNotReportBusy(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewJobRepository(db.Conn())
	ctx := context.Background()

	const writers, readers, iterations = 4, 8, 25

	var wg sync.WaitGroup
	errs := make(chan error, (writers+readers)*iterations)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				job := &domain.Job{
					CanonicalURL: fmt.Sprintf("https://acme.example/w%d-job-%d", w, i),
					Title:        "Backend Engineer",
					CompanyName:  "Acme",
				}
				if err := repo.Save(ctx, job); err != nil {
					errs <- fmt.Errorf("writer %d iteration %d: %w", w, i, err)
				}
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// The exact call JobDiscoveryService.Search makes per result.
				if _, err := repo.GetByCanonicalURL(ctx, fmt.Sprintf("https://acme.example/w0-job-%d", i)); err != nil {
					errs <- fmt.Errorf("reader %d iteration %d: %w", r, i, err)
				}
			}
		}(r)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		assert.NotContains(t, err.Error(), "SQLITE_BUSY")
		assert.NoError(t, err)
	}
}
