-- +goose Up
-- Named search criteria the user keeps and re-runs
-- (docs/02_features/04_job_discovery.md AC-001).
--
-- Criteria are explicit columns rather than a JSON blob: the field set is
-- deliberately small and fixed (see domain.SearchCriteria, which dropped the
-- fields no source could populate), and columns keep the schema honest about
-- what a search can actually filter on.
CREATE TABLE IF NOT EXISTS saved_searches (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	keywords TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL DEFAULT '',
	location TEXT NOT NULL DEFAULT '',
	remote_preference TEXT NOT NULL DEFAULT '',
	employment_type TEXT NOT NULL DEFAULT '',
	salary_min REAL NOT NULL DEFAULT 0,
	salary_max REAL NOT NULL DEFAULT 0,
	excluded_companies TEXT NOT NULL DEFAULT '',
	is_default INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- At most one default. A partial unique index enforces it in the schema
-- rather than leaving it to whichever code path happens to set the flag.
CREATE UNIQUE INDEX IF NOT EXISTS idx_saved_searches_single_default
	ON saved_searches (is_default) WHERE is_default = 1;

-- +goose Down
DROP INDEX IF EXISTS idx_saved_searches_single_default;
DROP TABLE IF EXISTS saved_searches;
