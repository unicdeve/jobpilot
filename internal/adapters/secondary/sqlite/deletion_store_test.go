//go:build integration

package sqlite

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

type noCredentials struct{}

func (noCredentials) List(context.Context) ([]domain.CredentialRef, error) { return nil, nil }
func (noCredentials) Delete(context.Context, string, string) error         { return nil }
func (noCredentials) FromEnvironment(domain.CredentialRef) bool            { return false }

// kindCounts counts every record by kind, the way a preview reports them.
func kindCounts(t *testing.T, db *DB) map[domain.RecordKind]int {
	t.Helper()
	counts := map[domain.RecordKind]int{}
	for table, kind := range recordTables {
		if n := count(t, db, `SELECT count(*) FROM `+table); n > 0 {
			counts[kind] += n
		}
	}
	return counts
}

// deletionGraph is connectedRows plus what the other targets need: a second
// job nothing uses, one marked not interested, an older cover letter, and a
// sent application.
var deletionGraph = append(append([]string{}, connectedRows...),
	`INSERT INTO jobs (id, canonical_url, title) VALUES ('unused-job', 'https://example.com/2', 'Designer')`,
	`INSERT INTO jobs (id, canonical_url, title, not_interested_at) VALUES ('ruled-out', 'https://example.com/3', 'Sales', '2026-09-25 10:00:00+00:00')`,
	`INSERT INTO cover_letters (id, application_id, content) VALUES ('old-letter', 'app', 'Dear Acme, earlier')`,
	`INSERT INTO jobs (id, canonical_url, title) VALUES ('sent-job', 'https://example.com/4', 'Backend')`,
	`INSERT INTO applications (id, job_id, candidate_id, state, submitted_at) VALUES ('sent-app', 'sent-job', 'cand', 'SUBMITTED', '2026-09-20 09:00:00+00:00')`,
	`INSERT INTO personal_answers (id, candidate_id, key, answer, created_at, updated_at) VALUES ('about', 'cand', 'values', 'Honesty', '2026-09-29', '2026-09-29')`,
	`INSERT INTO personal_answers (id, application_id, key, answer, created_at, updated_at) VALUES ('draws', 'app', 'draws_you', 'Tooling', '2026-09-29', '2026-09-29')`,
)

// TestDeletionStore_DeletesExactlyWhatThePreviewSaid is ADR-0021's central
// promise, on the real schema: for every target, the records that vanish
// are the ones the preview counted, kind for kind, and the ones it said
// would be detached are still there, unlinked.
func TestDeletionStore_DeletesExactlyWhatThePreviewSaid(t *testing.T) {
	t.Parallel()
	targets := []struct {
		target domain.DeletionTarget
		id     string
	}{
		{domain.DeleteJob, "job"},
		{domain.DeleteJob, "sent-job"},
		{domain.DeleteApplication, "app"},
		{domain.DeleteCoverLetter, "old-letter"},
		{domain.DeleteResume, "res"},
		{domain.DeleteProfile, ""},
		{domain.ClearActivity, ""},
		{domain.ClearJobs, ""},
	}
	for _, tc := range targets {
		t.Run(string(tc.target)+" "+tc.id, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := newTestDB(t)
			execAll(t, db, true, deletionGraph...)
			svc := services.NewDeletionService(NewDeletionStore(db), noCredentials{}, nil, nil)

			before := kindCounts(t, db)
			reach, err := svc.Preview(ctx, tc.target, tc.id)
			require.NoError(t, err)
			_, err = svc.Delete(ctx, tc.target, tc.id, reach.Token)
			require.NoError(t, err)
			after := kindCounts(t, db)

			vanished := map[domain.RecordKind]int{}
			for kind, n := range before {
				if gone := n - after[kind]; gone > 0 {
					vanished[kind] = gone
				}
			}
			assert.Equal(t, reach.Deleted, vanished, "what went is not what the preview said would go")
			assert.Zero(t, count(t, db, `SELECT count(*) FROM pragma_foreign_key_check`))
		})
	}
}

func TestDeletionStore_Specifics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("a resume's applications are counted as detached, and are", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, deletionGraph...)
		svc := services.NewDeletionService(NewDeletionStore(db), noCredentials{}, nil, nil)

		reach, err := svc.Preview(ctx, domain.DeleteResume, "res")
		require.NoError(t, err)
		assert.Equal(t, map[domain.RecordKind]int{domain.RecordApplication: 1}, reach.Detached)
		_, err = svc.Delete(ctx, domain.DeleteResume, "res", reach.Token)
		require.NoError(t, err)

		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM applications WHERE id = 'app' AND resume_id IS NULL`))
	})

	t.Run("clear jobs keeps jobs in use and ruled out, and counts the ruled out", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, deletionGraph...)
		svc := services.NewDeletionService(NewDeletionStore(db), noCredentials{}, nil, nil)

		reach, err := svc.Preview(ctx, domain.ClearJobs, "")
		require.NoError(t, err)

		assert.Equal(t, map[domain.RecordKind]int{domain.RecordJob: 1}, reach.Deleted, "only the unused job")
		assert.Equal(t, 1, reach.KeptNotInterested)
	})

	t.Run("the current cover letter is refused, and a sent application counted", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, deletionGraph...)
		svc := services.NewDeletionService(NewDeletionStore(db), noCredentials{}, nil, nil)

		_, err := svc.Preview(ctx, domain.DeleteCoverLetter, "letter")
		require.ErrorIs(t, err, services.ErrDeletionRefused)

		reach, err := svc.Preview(ctx, domain.DeleteJob, "sent-job")
		require.NoError(t, err)
		assert.Equal(t, 1, reach.SentApplications)
	})

	t.Run("delete all data leaves an empty database with its schema", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, deletionGraph...)
		execAll(t, db, true, `INSERT INTO user_settings (key, value) VALUES ('llm', '{}')`)
		svc := services.NewDeletionService(NewDeletionStore(db), noCredentials{}, nil, nil)

		schemaVersion := count(t, db, `SELECT max(version_id) FROM goose_db_version`)

		reach, err := svc.Preview(ctx, domain.DeleteEverything, "")
		require.NoError(t, err)
		assert.Equal(t, 4, reach.Deleted[domain.RecordJob])
		_, err = svc.Delete(ctx, domain.DeleteEverything, "", reach.Token)
		require.NoError(t, err)

		assert.Empty(t, kindCounts(t, db))
		assert.Equal(t, schemaVersion, count(t, db, `SELECT max(version_id) FROM goose_db_version`), "the schema stays")
	})
}

// TestDeletionStore_DeletedContentIsGoneFromDisk is AC-006: once a deletion
// returns, what it deleted cannot be read from the database file or its WAL.
func TestDeletionStore_DeletedContentIsGoneFromDisk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	path := dbPath(t, db)
	const marker = "JOBPILOT-RESUME-MARKER-7f3a9c"

	execAll(t, db, true,
		`INSERT INTO candidates (id, first_name) VALUES ('cand', 'Ada')`,
		`INSERT INTO resumes (id, candidate_id, name, markdown_content) VALUES ('res', 'cand', 'CV', '`+marker+`')`,
	)
	require.True(t, onDisk(t, path, marker), "the test must first see the marker on disk to prove anything")

	svc := services.NewDeletionService(NewDeletionStore(db), noCredentials{}, nil, nil)
	reach, err := svc.Preview(ctx, domain.DeleteResume, "res")
	require.NoError(t, err)
	result, err := svc.Delete(ctx, domain.DeleteResume, "res", reach.Token)
	require.NoError(t, err)
	require.Empty(t, result.Notices)

	assert.False(t, onDisk(t, path, marker), "the deleted resume can still be read from the files")
}

func dbPath(t *testing.T, db *DB) string {
	t.Helper()
	var seq int
	var name, file string
	require.NoError(t, db.Conn().QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &file))
	return file
}

// onDisk reports whether text appears in the database file or its WAL.
func onDisk(t *testing.T, path, text string) bool {
	t.Helper()
	for _, file := range []string{path, path + "-wal"} {
		content, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		if bytes.Contains(content, []byte(text)) {
			return true
		}
	}
	return false
}
