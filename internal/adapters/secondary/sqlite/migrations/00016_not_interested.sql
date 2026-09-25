-- +goose Up
-- "Not interested" (docs/02_features/15_data_deletion.md): the user has
-- ruled a job out. The job stays stored, so discovery still recognises it
-- by its canonical URL, and leaves the lists, a search's result and every
-- scout's consideration until "Interested again" clears the mark.
ALTER TABLE jobs ADD COLUMN not_interested_at DATETIME;

CREATE INDEX IF NOT EXISTS idx_jobs_not_interested_at ON jobs(not_interested_at);

-- +goose Down
DROP INDEX IF EXISTS idx_jobs_not_interested_at;
ALTER TABLE jobs DROP COLUMN not_interested_at;
