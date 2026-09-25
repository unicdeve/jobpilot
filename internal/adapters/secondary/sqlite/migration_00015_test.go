//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// migratedTo opens a fresh database migrated up to version.
func migratedTo(t *testing.T, version int64) *DB {
	t.Helper()
	db, err := NewDB(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	provider, err := db.migrationProvider()
	require.NoError(t, err)
	_, err = provider.UpTo(context.Background(), version)
	require.NoError(t, err)
	return db
}

func migrateUp(t *testing.T, db *DB) {
	t.Helper()
	provider, err := db.migrationProvider()
	require.NoError(t, err)
	_, err = provider.Up(context.Background())
	require.NoError(t, err)
}

// execAll runs statements on one connection with foreign keys off, which is
// how rows referencing deleted ones came to exist in the first place.
func execAll(t *testing.T, db *DB, foreignKeys bool, statements ...string) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn().Conn(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()

	if !foreignKeys {
		_, err = conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`)
		require.NoError(t, err)
		defer func() {
			_, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
			require.NoError(t, err)
		}()
	}
	for _, statement := range statements {
		_, err := conn.ExecContext(ctx, statement)
		require.NoError(t, err, statement)
	}
}

func count(t *testing.T, db *DB, query string) int {
	t.Helper()
	var n int
	require.NoError(t, db.Conn().QueryRow(query).Scan(&n), query)
	return n
}

// columnInfo is a table's columns exactly as SQLite describes them.
func columnInfo(t *testing.T, db *DB, table string) []string {
	t.Helper()
	rows, err := db.Conn().Query(
		`SELECT cid || '|' || name || '|' || type || '|' || "notnull" || '|' || ifnull(dflt_value, '<null>') || '|' || pk
		FROM pragma_table_info(?) ORDER BY cid`, table)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var columns []string
	for rows.Next() {
		var column string
		require.NoError(t, rows.Scan(&column))
		columns = append(columns, column)
	}
	require.NoError(t, rows.Err())
	return columns
}

// A graph of one of everything, with every reference satisfied.
var connectedRows = []string{
	`INSERT INTO candidates (id, first_name) VALUES ('cand', 'Ada')`,
	`INSERT INTO resumes (id, candidate_id, name) VALUES ('res', 'cand', 'CV')`,
	`INSERT INTO jobs (id, canonical_url, title) VALUES ('job', 'https://example.com/1', 'Engineer')`,
	`INSERT INTO job_matches (id, job_id, candidate_id, score) VALUES ('match', 'job', 'cand', 80)`,
	`INSERT INTO applications (id, job_id, candidate_id, resume_id, state) VALUES ('app', 'job', 'cand', 'res', 'READY_FOR_REVIEW')`,
	`INSERT INTO cover_letters (id, application_id, content) VALUES ('letter', 'app', 'Dear Acme')`,
	`UPDATE applications SET current_cover_letter_id = 'letter' WHERE id = 'app'`,
	`INSERT INTO application_questions (id, application_id, label) VALUES ('question', 'app', 'Email')`,
	`INSERT INTO application_state_transitions (id, application_id, to_state) VALUES ('transition', 'app', 'READY_FOR_REVIEW')`,
	`INSERT INTO saved_searches (id, name, keywords) VALUES ('search', 'Go', 'go')`,
	`INSERT INTO scouts (id, name, saved_search_id) VALUES ('scout', 'Daily', 'search')`,
	`INSERT INTO agent_runs (id, task_type, application_id) VALUES ('browser-run', 'prepare_application', 'app')`,
	`INSERT INTO agent_events (id, run_id, action) VALUES ('browser-step', 'browser-run', 'navigate')`,
	`INSERT INTO agent_runs (id, task_type, scout_id) VALUES ('scout-run', 'scout_run', 'scout')`,
	`INSERT INTO agent_events (id, run_id, action) VALUES ('scout-step', 'scout-run', 'scout_started')`,
}

// TestMigration00015_KeepsEveryColumnAndRow is ADR-0021 §5: the rebuild
// changes foreign key rules and indexes, and nothing else.
func TestMigration00015_KeepsEveryColumnAndRow(t *testing.T) {
	t.Parallel()
	db := migratedTo(t, 14)
	execAll(t, db, true, connectedRows...)

	before := map[string][]string{}
	rowsBefore := map[string]int{}
	for _, table := range deletionRuleTables {
		before[table.name] = columnInfo(t, db, table.name)
		rowsBefore[table.name] = count(t, db, `SELECT count(*) FROM `+table.name)
	}

	migrateUp(t, db)

	for _, table := range deletionRuleTables {
		assert.Equal(t, before[table.name], columnInfo(t, db, table.name), "%s changed shape", table.name)
		assert.Equal(t, rowsBefore[table.name], count(t, db, `SELECT count(*) FROM `+table.name), "%s lost rows", table.name)
	}
	assert.Equal(t, "letter", queryString(t, db, `SELECT current_cover_letter_id FROM applications WHERE id = 'app'`))
}

// TestMigration00015_RepairsOrphansByTheirRules: rows left referencing
// deleted ones, from when foreign keys were enforced on one pooled
// connection only, are given the effect their rule prescribes rather than
// stopping the upgrade.
func TestMigration00015_RepairsOrphansByTheirRules(t *testing.T) {
	t.Parallel()
	db := migratedTo(t, 14)
	execAll(t, db, true, connectedRows...)
	execAll(t, db, false,
		// A resume deleted from under an application: detach.
		`DELETE FROM resumes WHERE id = 'res'`,
		// A job deleted from under an application, which has a question of
		// its own: the application goes, and then, on the next pass, the
		// question that referenced it.
		`INSERT INTO applications (id, job_id, candidate_id) VALUES ('orphan-app', 'gone-job', 'cand')`,
		`INSERT INTO application_questions (id, application_id, label) VALUES ('orphan-question', 'orphan-app', 'Phone')`,
		// A step whose run is gone.
		`INSERT INTO agent_events (id, run_id, action) VALUES ('orphan-step', 'gone-run', 'navigate')`,
	)

	migrateUp(t, db)

	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM applications WHERE id = 'app' AND resume_id IS NULL`),
		"the application whose resume was deleted is kept and detached")
	assert.Zero(t, count(t, db, `SELECT count(*) FROM applications WHERE id = 'orphan-app'`))
	assert.Zero(t, count(t, db, `SELECT count(*) FROM application_questions WHERE id = 'orphan-question'`),
		"what the deleted orphan held goes too")
	assert.Zero(t, count(t, db, `SELECT count(*) FROM agent_events WHERE id = 'orphan-step'`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM application_questions WHERE id = 'question'`),
		"rows that referenced nothing missing are untouched")
	assert.Zero(t, count(t, db, `SELECT count(*) FROM pragma_foreign_key_check`))
}

// TestSchema_ForeignKeysAreTheDeletionRules is ADR-0021 §3: every foreign
// key's ON DELETE is the effect domain.DeletionRules declares, rule for
// rule, and every table is one whose records have a kind.
func TestSchema_ForeignKeysAreTheDeletionRules(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	rows, err := db.Conn().Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	require.NoError(t, err)
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	inSchema := map[domain.DeletionRule]bool{}
	for _, table := range names {
		if isInternalTable(table) {
			continue
		}
		referencing, ok := recordTables[table]
		require.True(t, ok, "table %s holds no kind of record; add it to recordTables", table)

		fks, err := db.Conn().Query(`SELECT "table", on_delete FROM pragma_foreign_key_list(?)`, table)
		require.NoError(t, err)
		for fks.Next() {
			var parent, onDelete string
			require.NoError(t, fks.Scan(&parent, &onDelete))
			referenced, ok := recordTables[parent]
			require.True(t, ok, "%s references %s, which holds no kind of record", table, parent)

			var effect domain.DeletionEffect
			switch onDelete {
			case "CASCADE":
				effect = domain.DeletionCascade
			case "SET NULL":
				effect = domain.DeletionDetach
			default:
				t.Errorf("%s references %s with ON DELETE %s, which blocks deleting it; declare a rule", table, parent, onDelete)
				continue
			}
			inSchema[domain.DeletionRule{Referencing: referencing, Referenced: referenced, Effect: effect}] = true
		}
		require.NoError(t, fks.Err())
		require.NoError(t, fks.Close())
	}

	declared := map[domain.DeletionRule]bool{}
	for _, rule := range domain.DeletionRules {
		declared[rule] = true
	}
	assert.Equal(t, declared, inSchema, "the schema's foreign keys and domain.DeletionRules disagree")
}

// TestSchema_DeletionFollowsTheRules deletes each kind of record the rules
// reach through and checks the effect, in SQL, of the rules the schema now
// carries. It includes the case that used to fail outright: a resume an
// application used.
func TestSchema_DeletionFollowsTheRules(t *testing.T) {
	t.Parallel()

	t.Run("a resume an application used is detached from it", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, connectedRows...)

		execAll(t, db, true, `DELETE FROM resumes WHERE id = 'res'`)

		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM applications WHERE id = 'app' AND resume_id IS NULL`))
	})

	t.Run("a job takes its match, applications and everything under them", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, connectedRows...)

		execAll(t, db, true, `DELETE FROM jobs WHERE id = 'job'`)

		for _, table := range []string{"job_matches", "applications", "cover_letters", "application_questions", "application_state_transitions"} {
			assert.Zero(t, count(t, db, `SELECT count(*) FROM `+table), table)
		}
		assert.Zero(t, count(t, db, `SELECT count(*) FROM agent_runs WHERE id = 'browser-run'`), "the browser run went with its application")
		assert.Zero(t, count(t, db, `SELECT count(*) FROM agent_events WHERE id = 'browser-step'`))
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM agent_runs WHERE id = 'scout-run'`), "a scout run is not the application's")
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM resumes`), "nor is the resume")
	})

	t.Run("a scout's runs outlive it, unlinked", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, connectedRows...)

		execAll(t, db, true, `DELETE FROM saved_searches WHERE id = 'search'`)

		assert.Zero(t, count(t, db, `SELECT count(*) FROM scouts`))
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM agent_runs WHERE id = 'scout-run' AND scout_id IS NULL`))
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM agent_events WHERE id = 'scout-step'`))
	})

	t.Run("the candidate takes everything about them, and nothing about the search", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		execAll(t, db, true, connectedRows...)

		execAll(t, db, true, `DELETE FROM candidates WHERE id = 'cand'`)

		for _, table := range []string{"resumes", "job_matches", "applications", "cover_letters", "application_questions"} {
			assert.Zero(t, count(t, db, `SELECT count(*) FROM `+table), table)
		}
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM jobs`))
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM scouts`))
	})
}

func queryString(t *testing.T, db *DB, query string) string {
	t.Helper()
	var value sql.NullString
	require.NoError(t, db.Conn().QueryRow(query).Scan(&value), query)
	return value.String
}

// isInternalTable reports whether a table belongs to SQLite or to the
// migration tool rather than holding JobPilot's records.
func isInternalTable(name string) bool {
	return name == "goose_db_version" || strings.HasPrefix(name, "sqlite_")
}
