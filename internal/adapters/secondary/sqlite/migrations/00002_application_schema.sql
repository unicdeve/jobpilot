-- +goose Up

-- Cover letters are versioned rather than a single mutable column on
-- applications, so regenerating one (docs/02_features/06_cover_letters.md
-- AC-004) keeps prior attempts instead of overwriting them, satisfying
-- AC-005's requirement that generated content be stored in application
-- history.
CREATE TABLE IF NOT EXISTS cover_letters (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	content TEXT,
	model_provider TEXT,
	model_name TEXT,
	user_edited BOOLEAN DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (application_id) REFERENCES applications(id)
);

CREATE INDEX IF NOT EXISTS idx_cover_letters_application_id ON cover_letters(application_id);

-- Durable history of application state changes
-- (docs/02_features/08_applications.md AC-003), independent of the
-- mutable `state` column on applications itself.
CREATE TABLE IF NOT EXISTS application_state_transitions (
	id TEXT PRIMARY KEY,
	application_id TEXT NOT NULL,
	from_state TEXT,
	to_state TEXT NOT NULL,
	reason TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (application_id) REFERENCES applications(id)
);

CREATE INDEX IF NOT EXISTS idx_application_state_transitions_application_id ON application_state_transitions(application_id);

ALTER TABLE applications ADD COLUMN current_cover_letter_id TEXT REFERENCES cover_letters(id);
ALTER TABLE applications DROP COLUMN cover_letter;

-- Records which of the four resolver categories in
-- docs/02_features/07_application_answers.md answered a question
-- (profile_fact | derived | generated | needs_user_input), distinct from
-- answer_source's free-text detail (e.g. which profile field, or which
-- derivation rule, produced the answer).
ALTER TABLE application_questions ADD COLUMN category TEXT;

-- +goose Down
ALTER TABLE application_questions DROP COLUMN category;

ALTER TABLE applications ADD COLUMN cover_letter TEXT;
ALTER TABLE applications DROP COLUMN current_cover_letter_id;

DROP INDEX IF EXISTS idx_application_state_transitions_application_id;
DROP TABLE IF EXISTS application_state_transitions;

DROP INDEX IF EXISTS idx_cover_letters_application_id;
DROP TABLE IF EXISTS cover_letters;
