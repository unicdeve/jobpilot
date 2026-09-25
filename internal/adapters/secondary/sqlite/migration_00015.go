package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// rebuiltTable is one table 00015 recreates: its final definition and the
// indexes it carries.
type rebuiltTable struct {
	name    string
	create  string
	indexes []string
}

// deletionRuleTables are the eight tables whose foreign keys had no ON
// DELETE rule, written out with the rules of domain.DeletionRules
// (ADR-0021 §3, §5). Columns, types, defaults and keys are exactly what
// migrations 00001 to 00014 left, in the same order; only the foreign
// keys' rules and the indexes change.
//
// Every referencing column gets an index. SQLite finds the rows a cascade
// reaches by scanning the referencing table without one.
var deletionRuleTables = []rebuiltTable{
	{
		name: "resumes",
		create: `CREATE TABLE resumes (
	id TEXT PRIMARY KEY,
	candidate_id TEXT NOT NULL,
	name TEXT NOT NULL,
	original_file_path TEXT,
	markdown_content TEXT,
	parsed_metadata TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE
)`,
		indexes: []string{
			`CREATE INDEX idx_resumes_candidate_id ON resumes(candidate_id)`,
		},
	},
	{
		name: "job_matches",
		create: `CREATE TABLE job_matches (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	candidate_id TEXT NOT NULL,
	score REAL,
	confidence REAL,
	strengths TEXT,
	gaps TEXT,
	concerns TEXT,
	matched_skills TEXT,
	missing_skills TEXT,
	rationale TEXT,
	model_provider TEXT,
	model_name TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
	FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE
)`,
		indexes: []string{
			`CREATE INDEX idx_job_matches_job_id ON job_matches(job_id)`,
			`CREATE INDEX idx_job_matches_candidate_id ON job_matches(candidate_id)`,
		},
	},
	{
		name: "applications",
		create: `CREATE TABLE applications (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	candidate_id TEXT NOT NULL,
	resume_id TEXT,
	mode TEXT,
	state TEXT,
	questions TEXT,
	policy_result TEXT,
	approval_status TEXT,
	submitted_at DATETIME,
	failure_reason TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	current_cover_letter_id TEXT REFERENCES cover_letters(id) ON DELETE SET NULL,
	FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
	FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE,
	FOREIGN KEY (resume_id) REFERENCES resumes(id) ON DELETE SET NULL
)`,
		indexes: []string{
			`CREATE INDEX idx_applications_job_id ON applications(job_id)`,
			`CREATE INDEX idx_applications_candidate_id ON applications(candidate_id)`,
			`CREATE INDEX idx_applications_resume_id ON applications(resume_id)`,
			`CREATE INDEX idx_applications_current_cover_letter_id ON applications(current_cover_letter_id)`,
		},
	},
	{
		name: "application_questions",
		create: `CREATE TABLE application_questions (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	label TEXT NOT NULL,
	type TEXT,
	required BOOLEAN,
	answer TEXT,
	answer_source TEXT,
	confidence REAL,
	user_edited BOOLEAN,
	status TEXT,
	category TEXT,
	source TEXT NOT NULL DEFAULT 'manual',
	field_ref TEXT,
	FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE
)`,
		indexes: []string{
			`CREATE INDEX idx_application_questions_application_id ON application_questions(application_id)`,
		},
	},
	{
		name: "cover_letters",
		create: `CREATE TABLE cover_letters (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	content TEXT,
	model_provider TEXT,
	model_name TEXT,
	user_edited BOOLEAN DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE
)`,
		indexes: []string{
			`CREATE INDEX idx_cover_letters_application_id ON cover_letters(application_id)`,
		},
	},
	{
		name: "application_state_transitions",
		create: `CREATE TABLE application_state_transitions (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	from_state TEXT,
	to_state TEXT NOT NULL,
	reason TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE
)`,
		indexes: []string{
			`CREATE INDEX idx_application_state_transitions_application_id ON application_state_transitions(application_id)`,
		},
	},
	{
		name: "agent_runs",
		create: `CREATE TABLE agent_runs (
	id TEXT PRIMARY KEY,
	task_type TEXT,
	status TEXT,
	started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	completed_at DATETIME,
	error TEXT,
	application_id TEXT REFERENCES applications(id) ON DELETE CASCADE,
	scout_id TEXT REFERENCES scouts(id) ON DELETE SET NULL
)`,
		indexes: []string{
			`CREATE INDEX idx_agent_runs_application_id ON agent_runs(application_id)`,
			`CREATE INDEX idx_agent_runs_task_type ON agent_runs(task_type)`,
			`CREATE INDEX idx_agent_runs_started_at ON agent_runs(started_at)`,
			`CREATE INDEX idx_agent_runs_scout_id ON agent_runs(scout_id)`,
		},
	},
	{
		name: "agent_events",
		create: `CREATE TABLE agent_events (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	action TEXT,
	tool TEXT,
	status TEXT,
	input_summary TEXT,
	output_summary TEXT,
	policy_result TEXT,
	duration_ms INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
)`,
		indexes: []string{
			`CREATE INDEX idx_agent_events_run_id ON agent_events(run_id)`,
		},
	},
}

// maxOrphanRepairPasses bounds the orphan repair. Deleting an orphan can
// orphan its own children, so repair runs in passes; the longest chain of
// references is four deep (a step, its run, its application, its job), so
// more passes than that means something is wrong rather than unfinished.
const maxOrphanRepairPasses = 8

// installDeletionRules is migration 00015 (ADR-0021 §5): it rebuilds the
// tables in deletionRuleTables so their foreign keys carry the deletion
// rules, using SQLite's documented procedure for changing a table's
// schema.
//
// That procedure needs foreign keys off, which cannot be changed inside a
// transaction, so this runs outside goose's and holds one connection
// throughout: off, rebuild in a transaction of its own, repair orphans,
// check, commit, on.
func installDeletionRules(ctx context.Context, db *sql.DB) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to take a connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("failed to turn foreign keys off: %w", err)
	}
	// However this ends, the connection must not go back to the pool with
	// foreign keys off: it would enforce nothing for as long as it lived.
	// If they cannot be turned back on, the connection is discarded.
	defer func() {
		if _, onErr := conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`); onErr != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("failed to turn foreign keys back on: %w", onErr))
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, table := range deletionRuleTables {
		if err := rebuildTable(ctx, tx, table); err != nil {
			return err
		}
	}
	if err := repairOrphans(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// rebuildTable replaces a table with its new definition, keeping every row.
func rebuildTable(ctx context.Context, tx *sql.Tx, table rebuiltTable) error {
	columns, err := columnList(ctx, tx, table.name)
	if err != nil {
		return err
	}

	staging := table.name + "_00015"
	statements := []string{
		replaceTableName(table.create, table.name, staging),
		fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s`, staging, columns, columns, table.name),
		fmt.Sprintf(`DROP TABLE %s`, table.name),
		fmt.Sprintf(`ALTER TABLE %s RENAME TO %s`, staging, table.name),
	}
	statements = append(statements, table.indexes...)
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to rebuild %s: %w", table.name, err)
		}
	}
	return nil
}

// columnList is a table's columns, in order, as a SQL list.
func columnList(ctx context.Context, tx *sql.Tx, table string) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return "", fmt.Errorf("failed to read the columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", fmt.Errorf("failed to read the columns of %s: %w", table, err)
		}
		names = append(names, quoteIdent(name))
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("failed to read the columns of %s: %w", table, err)
	}
	if len(names) == 0 {
		return "", fmt.Errorf("table %s has no columns", table)
	}
	return strings.Join(names, ", "), nil
}

// replaceTableName rewrites "CREATE TABLE name (" to use another name.
func replaceTableName(create, name, replacement string) string {
	return strings.Replace(create, "CREATE TABLE "+name+" (", "CREATE TABLE "+replacement+" (", 1)
}

// orphan is a row SQLite reports as referencing a row that does not exist.
type orphan struct {
	table string
	rowid int64
	fkid  int
}

// repairOrphans gives each orphan the effect its rule prescribes — a row
// whose foreign key cascades is deleted, since what it was derived from is
// already gone, and a reference that is set null is cleared — and repeats
// until none remain (ADR-0021 §5).
//
// Orphans exist because foreign keys were once enforced only on the one
// pooled connection that happened to run the pragma. Refusing to migrate
// over them would stop the daemon starting, over data nobody asked to keep.
func repairOrphans(ctx context.Context, tx *sql.Tx) error {
	repaired := map[string]int{}
	for range maxOrphanRepairPasses {
		orphans, err := foreignKeyViolations(ctx, tx)
		if err != nil {
			return err
		}
		if len(orphans) == 0 {
			if len(repaired) > 0 {
				slog.InfoContext(ctx, "migration 00015 repaired rows that referenced deleted ones", "rows_by_table", repaired)
			}
			return nil
		}
		for _, o := range orphans {
			if err := repairOrphan(ctx, tx, o); err != nil {
				return err
			}
			repaired[o.table]++
		}
	}
	return fmt.Errorf("rows still reference deleted ones after %d repair passes", maxOrphanRepairPasses)
}

func foreignKeyViolations(ctx context.Context, tx *sql.Tx) ([]orphan, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, fmt.Errorf("failed to check foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orphans []orphan
	for rows.Next() {
		var o orphan
		var parent string
		var rowid sql.NullInt64
		if err := rows.Scan(&o.table, &rowid, &parent, &o.fkid); err != nil {
			return nil, fmt.Errorf("failed to read a foreign key violation: %w", err)
		}
		if !rowid.Valid {
			// Only a WITHOUT ROWID table reports no rowid, and JobPilot has none.
			return nil, fmt.Errorf("%s holds a row referencing a missing %s that cannot be addressed", o.table, parent)
		}
		o.rowid = rowid.Int64
		orphans = append(orphans, o)
	}
	return orphans, rows.Err()
}

func repairOrphan(ctx context.Context, tx *sql.Tx, o orphan) error {
	var column, onDelete string
	err := tx.QueryRowContext(ctx,
		`SELECT "from", on_delete FROM pragma_foreign_key_list(?) WHERE id = ?`, o.table, o.fkid).Scan(&column, &onDelete)
	if err != nil {
		return fmt.Errorf("failed to read foreign key %d of %s: %w", o.fkid, o.table, err)
	}

	var statement string
	switch onDelete {
	case "CASCADE":
		statement = fmt.Sprintf(`DELETE FROM %s WHERE rowid = ?`, quoteIdent(o.table))
	case "SET NULL":
		statement = fmt.Sprintf(`UPDATE %s SET %s = NULL WHERE rowid = ?`, quoteIdent(o.table), quoteIdent(column))
	default:
		return fmt.Errorf("%s.%s references a missing row and its foreign key has no rule to repair it by", o.table, column)
	}
	if _, err := tx.ExecContext(ctx, statement, o.rowid); err != nil {
		return fmt.Errorf("failed to repair a row of %s: %w", o.table, err)
	}
	return nil
}

// quoteIdent quotes a name from the schema as a SQL identifier.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
