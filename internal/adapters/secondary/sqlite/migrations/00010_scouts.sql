-- +goose Up
-- Phase 8: a Scout is a standing instruction that runs a saved search,
-- matches what it finds, and creates applications for the user to review
-- (ADR-0014, ADR-0018). It never contacts an employer and never submits.
--
-- The rules live in typed columns rather than a serialized rule language.
-- Four columns can be read in a SELECT, constrained by the schema and
-- checked by the compiler; the `rule TEXT` blob the policies table was
-- shaped for could be none of those.
--
-- Salary floor, excluded companies, location and work arrangement are
-- deliberately absent: saved_searches already holds every one of them and
-- SearchCriteria.Matches already applies them. A second copy here would be
-- a second place to change a rule, with no answer for which one wins
-- (ADR-0018 §1, following ADR-0015 §4).
CREATE TABLE IF NOT EXISTS scouts (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	-- ON DELETE CASCADE: a scout without its search has no criteria at
	-- all, and one that answered "everything matches" instead would be the
	-- worst failure available to it.
	saved_search_id TEXT NOT NULL REFERENCES saved_searches(id) ON DELETE CASCADE,
	min_match_score INTEGER NOT NULL DEFAULT 0,
	max_applications_per_run INTEGER NOT NULL DEFAULT 10,
	-- 'local' considers jobs already stored; 'sources' fetches from the
	-- configured job boards first. Both stop before the browser.
	source TEXT NOT NULL DEFAULT 'local',
	is_enabled INTEGER NOT NULL DEFAULT 1,
	last_run_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scouts_saved_search_id ON scouts(saved_search_id);

-- A scout run is an agent run belonging to no single application, so runs
-- are read back by the absence of one.
CREATE INDEX IF NOT EXISTS idx_agent_runs_task_type ON agent_runs(task_type);

-- The policies table was created by migration 00001 and has never been
-- read or written. Its `rule TEXT` column was a placeholder for a decision
-- nobody had made yet; ADR-0018 makes it, and the answer is four typed
-- columns above rather than a blob. Keeping an empty table that looks like
-- it does something is what ADR-0015's closing change removed three
-- columns for.
DROP TABLE IF EXISTS policies;

-- +goose Down
CREATE TABLE IF NOT EXISTS policies (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	rule TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

DROP INDEX IF EXISTS idx_agent_runs_task_type;
DROP INDEX IF EXISTS idx_scouts_saved_search_id;
DROP TABLE IF EXISTS scouts;
