package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// candidateWrittenTables are the tables migration 00026 recreates without
// what only JobPilot's own writing and browser preparation used
// (ADR-0034): each table's final definition, the columns kept from the
// old one, and its indexes. A table whose rows all go is listed with no
// kept columns and is recreated empty.
var candidateWrittenTables = []struct {
	rebuiltTable
	kept string
}{
	{
		rebuiltTable: rebuiltTable{
			name: "applications",
			create: `CREATE TABLE applications (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	candidate_id TEXT NOT NULL,
	resume_id TEXT,
	state TEXT,
	submitted_at DATETIME,
	current_cover_letter_id TEXT REFERENCES cover_letters(id) ON DELETE SET NULL,
	apply_url TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
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
		kept: `id, job_id, candidate_id, resume_id, state, submitted_at, current_cover_letter_id, apply_url, created_at, updated_at`,
	},
	{
		rebuiltTable: rebuiltTable{
			name: "application_questions",
			create: `CREATE TABLE application_questions (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	label TEXT NOT NULL,
	answer TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE
)`,
			indexes: []string{
				`CREATE INDEX idx_application_questions_application_id ON application_questions(application_id)`,
			},
		},
	},
	{
		rebuiltTable: rebuiltTable{
			name: "cover_letters",
			create: `CREATE TABLE cover_letters (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	content TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (application_id) REFERENCES applications(id) ON DELETE CASCADE
)`,
			indexes: []string{
				`CREATE INDEX idx_cover_letters_application_id ON cover_letters(application_id)`,
			},
		},
	},
	{
		rebuiltTable: rebuiltTable{
			name: "agent_runs",
			create: `CREATE TABLE agent_runs (
	id TEXT PRIMARY KEY,
	task_type TEXT,
	status TEXT,
	started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	completed_at DATETIME,
	error TEXT,
	scout_id TEXT REFERENCES scouts(id) ON DELETE SET NULL,
	started_by TEXT NOT NULL DEFAULT 'manual'
)`,
			indexes: []string{
				`CREATE INDEX idx_agent_runs_task_type ON agent_runs(task_type)`,
				`CREATE INDEX idx_agent_runs_started_at ON agent_runs(started_at)`,
				`CREATE INDEX idx_agent_runs_scout_id ON agent_runs(scout_id)`,
				`CREATE INDEX idx_agent_runs_scout_started ON agent_runs(scout_id, started_at)`,
			},
		},
		kept: `id, task_type, status, started_at, completed_at, error, scout_id, started_by`,
	},
	{
		rebuiltTable: rebuiltTable{
			name: "personal_answers",
			create: `CREATE TABLE personal_answers (
	id TEXT PRIMARY KEY,
	candidate_id TEXT NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
	key TEXT NOT NULL,
	question TEXT NOT NULL DEFAULT '',
	answer TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL
)`,
			indexes: []string{
				`CREATE INDEX idx_personal_answers_candidate ON personal_answers(candidate_id)`,
				// One answer per standing question; any number kept.
				`CREATE UNIQUE INDEX idx_personal_answers_candidate_key
    ON personal_answers(candidate_id, key)
    WHERE key <> 'kept'`,
			},
		},
		kept: `id, candidate_id, key, question, answer, created_at, updated_at`,
	},
}

// preparationStates are the states that existed only for browser
// preparation, or for a run of it that failed (ADR-0034 §3).
var preparationStates = []string{"PREPARING", "READY_FOR_REVIEW", "FAILED"}

// removedPersonalAnswerKeys are the About you answers that only steered
// the model (ADR-0034 §5).
var removedPersonalAnswerKeys = []string{"writing_sample", "writing_preferences", "never_mention"}

// candidateWritesTheirOwn is migration 00026 (ADR-0034 §6). It deletes what
// JobPilot wrote or extracted — every cover letter, every application
// question and answer, every browser preparation run with its steps, the
// About you answers that only steered the model and every answer kept for
// one application — moves each application left in a preparation state
// back to MATCHED with a history entry saying why, drops the guidance for
// the prompts that no longer exist, and recreates the tables without the
// columns only those used.
//
// Recreating a table needs foreign keys off, which cannot change inside a
// transaction, so it runs outside goose's as 00015 does.
func candidateWritesTheirOwn(ctx context.Context, db *sql.DB) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to take a connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("failed to turn foreign keys off: %w", err)
	}
	// The connection must not go back to the pool with foreign keys off;
	// one that cannot turn them back on is discarded.
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

	if err := deleteGeneratedContent(ctx, tx); err != nil {
		return err
	}
	if err := returnPreparedApplications(ctx, tx, time.Now().UTC()); err != nil {
		return err
	}
	for _, table := range candidateWrittenTables {
		if err := recreateTable(ctx, tx, table.rebuiltTable, table.kept); err != nil {
			return err
		}
	}

	violations, err := foreignKeyViolations(ctx, tx)
	if err != nil {
		return err
	}
	if len(violations) > 0 {
		return fmt.Errorf("recreating the tables left %d rows referencing rows that do not exist, the first in %s",
			len(violations), violations[0].table)
	}
	return tx.Commit()
}

func deleteGeneratedContent(ctx context.Context, tx *sql.Tx) error {
	removedKeys := make([]any, 0, len(removedPersonalAnswerKeys))
	for _, key := range removedPersonalAnswerKeys {
		removedKeys = append(removedKeys, key)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{query: `UPDATE applications SET current_cover_letter_id = NULL`},
		{query: `DELETE FROM cover_letters`},
		{query: `DELETE FROM application_questions`},
		// A notification about a run outlives it (ADR-0028 §4).
		{query: `UPDATE notifications SET run_id = NULL
			WHERE run_id IN (SELECT id FROM agent_runs WHERE task_type = 'prepare_application')`},
		{query: `DELETE FROM agent_events
			WHERE run_id IN (SELECT id FROM agent_runs WHERE task_type = 'prepare_application')`},
		{query: `DELETE FROM agent_runs WHERE task_type = 'prepare_application'`},
		{query: `DELETE FROM personal_answers WHERE application_id IS NOT NULL`},
		{
			query: `DELETE FROM personal_answers WHERE key IN (` + placeholders(len(removedKeys)) + `)`,
			args:  removedKeys,
		},
		{query: `UPDATE user_settings SET value = json_remove(value, '$.cover_letter', '$.written_answer')
			WHERE key = 'prompt_guidance' AND json_valid(value)`},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("failed to delete generated content: %w", err)
		}
	}
	return nil
}

// returnPreparedApplications moves every application in a preparation
// state to MATCHED, recording the move in its history, so none disappears
// from the user's list and no outcome is invented.
func returnPreparedApplications(ctx context.Context, tx *sql.Tx, now time.Time) error {
	args := make([]any, 0, len(preparationStates))
	for _, state := range preparationStates {
		args = append(args, state)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, state FROM applications WHERE state IN (`+placeholders(len(preparationStates))+`)`, args...)
	if err != nil {
		return fmt.Errorf("failed to find applications being prepared: %w", err)
	}
	type moved struct{ id, from string }
	var applications []moved
	for rows.Next() {
		var application moved
		if err := rows.Scan(&application.id, &application.from); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to read an application being prepared: %w", err)
		}
		applications = append(applications, application)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return fmt.Errorf("failed to read applications being prepared: %w", err)
	}

	const reason = "JobPilot no longer prepares applications in a browser, so this one moved back to matched."
	for _, application := range applications {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO application_state_transitions (id, application_id, from_state, to_state, reason, created_at)
			VALUES (?, ?, ?, 'MATCHED', ?, ?)`,
			uuid.NewString(), application.id, application.from, reason, now,
		); err != nil {
			return fmt.Errorf("failed to record an application's move to matched: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE applications SET state = 'MATCHED', updated_at = ? WHERE id = ?`, now, application.id,
		); err != nil {
			return fmt.Errorf("failed to move an application to matched: %w", err)
		}
	}
	return nil
}

// recreateTable replaces a table with its new definition, copying the kept
// columns, or none.
func recreateTable(ctx context.Context, tx *sql.Tx, table rebuiltTable, kept string) error {
	staging := table.name + "_00026"
	statements := []string{replaceTableName(table.create, table.name, staging)}
	if kept != "" {
		statements = append(statements,
			fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s`, staging, kept, kept, table.name))
	}
	statements = append(statements,
		fmt.Sprintf(`DROP TABLE %s`, table.name),
		fmt.Sprintf(`ALTER TABLE %s RENAME TO %s`, staging, table.name),
	)
	statements = append(statements, table.indexes...)
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to recreate %s: %w", table.name, err)
		}
	}
	return nil
}
