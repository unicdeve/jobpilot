//go:build integration

package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generatedRowsAt25 is what an application JobPilot prepared held before
// ADR-0034: a generated letter, extracted questions, a browser run with its
// steps and a notification about it, About you answers that only steered
// the model, and answers given for one application. Beside it are what the
// candidate keeps: an application they sent, a scout run, their standing
// and kept answers, and their matching guidance.
var generatedRowsAt25 = []string{
	`INSERT INTO candidates (id, first_name) VALUES ('cand', 'Ada')`,
	`INSERT INTO jobs (id, canonical_url, title) VALUES ('job-1', 'https://example.com/1', 'Engineer')`,
	`INSERT INTO jobs (id, canonical_url, title) VALUES ('job-2', 'https://example.com/2', 'Designer')`,
	`INSERT INTO jobs (id, canonical_url, title) VALUES ('job-3', 'https://example.com/3', 'Writer')`,
	`INSERT INTO jobs (id, canonical_url, title) VALUES ('job-4', 'https://example.com/4', 'Backend')`,
	`INSERT INTO applications (id, job_id, candidate_id, mode, state, failure_reason, apply_url)
		VALUES ('preparing', 'job-1', 'cand', 'review', 'PREPARING', '', 'https://jobs.example/apply')`,
	`INSERT INTO applications (id, job_id, candidate_id, mode, state) VALUES ('ready', 'job-2', 'cand', 'review', 'READY_FOR_REVIEW')`,
	`INSERT INTO applications (id, job_id, candidate_id, mode, state, failure_reason)
		VALUES ('failed', 'job-3', 'cand', 'assist', 'FAILED', 'no apply control')`,
	`INSERT INTO applications (id, job_id, candidate_id, mode, state, submitted_at)
		VALUES ('sent', 'job-4', 'cand', 'review', 'INTERVIEWING', '2026-10-01 09:00:00+00:00')`,
	`INSERT INTO application_state_transitions (id, application_id, from_state, to_state) VALUES ('t-sent', 'sent', 'SUBMITTED', 'INTERVIEWING')`,
	`INSERT INTO cover_letters (id, application_id, content, model_provider, model_name) VALUES ('letter', 'ready', 'I am excited to apply', 'ollama', 'llama3.2')`,
	`UPDATE applications SET current_cover_letter_id = 'letter' WHERE id = 'ready'`,
	`INSERT INTO application_questions (id, application_id, label, type, answer, category, source, field_ref)
		VALUES ('question', 'ready', 'Why us?', 'narrative', 'Because', 'generated', 'form_extracted', 'f1')`,
	`INSERT INTO saved_searches (id, name, keywords) VALUES ('search', 'Go', 'go')`,
	`INSERT INTO scouts (id, name, saved_search_id) VALUES ('scout', 'Daily', 'search')`,
	`INSERT INTO agent_runs (id, task_type, status, application_id) VALUES ('browser-run', 'prepare_application', 'blocked', 'ready')`,
	`INSERT INTO agent_events (id, run_id, action) VALUES ('browser-step', 'browser-run', 'navigate')`,
	`INSERT INTO agent_runs (id, task_type, status, scout_id, started_by) VALUES ('scout-run', 'scout_run', 'completed', 'scout', 'schedule')`,
	`INSERT INTO agent_events (id, run_id, action) VALUES ('scout-step', 'scout-run', 'scout_started')`,
	`INSERT INTO notifications (id, kind, title, run_id, created_at) VALUES ('told', 'scout_found', 'Found 2', 'browser-run', '2026-10-01 09:00:00+00:00')`,
	`INSERT INTO personal_answers (id, candidate_id, key, question, answer, created_at, updated_at)
		VALUES ('values', 'cand', 'values', 'What do you care about?', 'Honesty', '2026-09-29', '2026-09-29')`,
	`INSERT INTO personal_answers (id, candidate_id, key, question, answer, created_at, updated_at)
		VALUES ('kept', 'cand', 'kept', 'Why us?', 'The tooling', '2026-09-29', '2026-09-29')`,
	`INSERT INTO personal_answers (id, candidate_id, key, answer, created_at, updated_at)
		VALUES ('sample', 'cand', 'writing_sample', 'Hi all', '2026-09-29', '2026-09-29')`,
	`INSERT INTO personal_answers (id, candidate_id, key, answer, created_at, updated_at)
		VALUES ('prefs', 'cand', 'writing_preferences', 'Warm', '2026-09-29', '2026-09-29')`,
	`INSERT INTO personal_answers (id, candidate_id, key, answer, created_at, updated_at)
		VALUES ('never', 'cand', 'never_mention', 'My gap year', '2026-09-29', '2026-09-29')`,
	`INSERT INTO personal_answers (id, application_id, key, question, answer, created_at, updated_at)
		VALUES ('draws', 'ready', 'draws_you', 'What draws you?', 'Tooling', '2026-09-29', '2026-09-29')`,
	`INSERT INTO user_settings (key, value) VALUES ('prompt_guidance',
		'{"match":{"text":"Weigh Go","default_digest":"d"},"cover_letter":{"text":"Short","default_digest":"d"},"written_answer":{"text":"Brief","default_digest":"d"}}')`,
}

// TestMigration00026_KeepsOnlyWhatTheCandidateWrote is ADR-0034 §6.
func TestMigration00026_KeepsOnlyWhatTheCandidateWrote(t *testing.T) {
	t.Parallel()
	db := migratedTo(t, 25)
	execAll(t, db, true, generatedRowsAt25...)

	migrateUpTo(t, db, 26)

	t.Run("what JobPilot wrote or extracted is gone", func(t *testing.T) {
		assert.Zero(t, count(t, db, `SELECT count(*) FROM cover_letters`))
		assert.Zero(t, count(t, db, `SELECT count(*) FROM application_questions`))
		assert.Zero(t, count(t, db, `SELECT count(*) FROM agent_runs WHERE id = 'browser-run'`))
		assert.Zero(t, count(t, db, `SELECT count(*) FROM agent_events WHERE id = 'browser-step'`))
		assert.Zero(t, count(t, db, `SELECT count(*) FROM applications WHERE current_cover_letter_id IS NOT NULL`))
	})

	t.Run("a notification about a removed run is kept, unlinked", func(t *testing.T) {
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM notifications WHERE id = 'told' AND run_id IS NULL`))
	})

	t.Run("applications being prepared move back to matched, with the reason in their history", func(t *testing.T) {
		for _, application := range []struct{ id, from string }{
			{"preparing", "PREPARING"}, {"ready", "READY_FOR_REVIEW"}, {"failed", "FAILED"},
		} {
			assert.Equal(t, "MATCHED", queryString(t, db, `SELECT state FROM applications WHERE id = '`+application.id+`'`))
			assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM application_state_transitions
				WHERE application_id = '`+application.id+`' AND from_state = '`+application.from+`' AND to_state = 'MATCHED'
				AND reason <> ''`), application.id)
		}
		assert.Equal(t, "https://jobs.example/apply", queryString(t, db, `SELECT apply_url FROM applications WHERE id = 'preparing'`),
			"the user's own link is kept")
	})

	t.Run("a sent application, its history and the scout's run are untouched", func(t *testing.T) {
		assert.Equal(t, "INTERVIEWING", queryString(t, db, `SELECT state FROM applications WHERE id = 'sent'`))
		assert.NotEmpty(t, queryString(t, db, `SELECT submitted_at FROM applications WHERE id = 'sent'`))
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM application_state_transitions WHERE id = 't-sent'`))
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM agent_runs WHERE id = 'scout-run' AND scout_id = 'scout' AND started_by = 'schedule'`))
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM agent_events WHERE id = 'scout-step'`))
	})

	t.Run("About you keeps the candidate's own answers only", func(t *testing.T) {
		var ids []string
		rows, err := db.Conn().Query(`SELECT id FROM personal_answers ORDER BY id`)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id string
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		assert.Equal(t, []string{"kept", "values"}, ids)
	})

	t.Run("guidance for removed prompts is dropped and matching's kept", func(t *testing.T) {
		assert.Equal(t, "Weigh Go", queryString(t, db,
			`SELECT json_extract(value, '$.match.text') FROM user_settings WHERE key = 'prompt_guidance'`))
		assert.Zero(t, count(t, db, `SELECT count(*) FROM user_settings
			WHERE key = 'prompt_guidance' AND (json_extract(value, '$.cover_letter') IS NOT NULL OR json_extract(value, '$.written_answer') IS NOT NULL)`))
	})

	t.Run("the removed columns are gone", func(t *testing.T) {
		for table, column := range map[string]string{
			"applications":          "mode",
			"cover_letters":         "model_name",
			"application_questions": "field_ref",
			"agent_runs":            "application_id",
			"personal_answers":      "application_id",
		} {
			assert.Zero(t, count(t, db, `SELECT count(*) FROM pragma_table_info('`+table+`') WHERE name = '`+column+`'`), "%s.%s", table, column)
		}
		for _, column := range []string{"failure_reason", "policy_result", "approval_status", "questions"} {
			assert.Zero(t, count(t, db, `SELECT count(*) FROM pragma_table_info('applications') WHERE name = '`+column+`'`), column)
		}
		assert.Zero(t, count(t, db, `SELECT count(*) FROM pragma_foreign_key_check`))
	})
}
