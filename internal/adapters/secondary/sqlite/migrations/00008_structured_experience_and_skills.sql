-- +goose Up
-- Stage two of ADR-0015: the candidate's history as entries rather than
-- prose.
--
-- Stage one fixed answers that were wrong. This one makes answers possible
-- that could not be given at all:
--
--   "Current company"  asked by two of the three recorded forms (Ashby and
--                      Lever), and unanswerable while the work history was
--                      a paragraph
--   years of experience
--                      comparable with jobs.experience_min/experience_max,
--                      which have existed since the first migration and
--                      have never been used for anything
--   skills             jobs.skills is already a list; matching has been
--                      comparing it against a blob of text
--
-- Education and certifications are deliberately not here. No recorded form
-- asks about a degree, an institution or a certification, and nothing in
-- the codebase reads either column — they are written and never used.
-- Schema for a requirement nobody has stated is the thing principle 10
-- rules out, so they stay prose until a form asks.

CREATE TABLE IF NOT EXISTS candidate_experiences (
	id TEXT PRIMARY KEY,
	candidate_id TEXT NOT NULL,
	company TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL DEFAULT '',
	-- Dates are nullable because a half-remembered history is still worth
	-- storing: an entry with no start date simply does not contribute to a
	-- computed span.
	start_date DATE,
	end_date DATE,
	is_current INTEGER NOT NULL DEFAULT 0,
	description TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS candidate_skills (
	id TEXT PRIMARY KEY,
	candidate_id TEXT NOT NULL,
	name TEXT NOT NULL,
	-- Nullable: "I know Go" and "I have used Go for four years" are
	-- different statements, and only the candidate can make the second.
	years_experience REAL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE,
	UNIQUE (candidate_id, name)
);

CREATE INDEX IF NOT EXISTS idx_candidate_experiences_candidate ON candidate_experiences (candidate_id);
CREATE INDEX IF NOT EXISTS idx_candidate_skills_candidate ON candidate_skills (candidate_id);

ALTER TABLE candidates DROP COLUMN skills;
ALTER TABLE candidates DROP COLUMN experiences;

-- +goose Down
ALTER TABLE candidates ADD COLUMN experiences TEXT;
ALTER TABLE candidates ADD COLUMN skills TEXT;

DROP INDEX IF EXISTS idx_candidate_skills_candidate;
DROP INDEX IF EXISTS idx_candidate_experiences_candidate;
DROP TABLE IF EXISTS candidate_skills;
DROP TABLE IF EXISTS candidate_experiences;
