-- +goose Up
-- When each saved search last ran against the job sources, so the desktop
-- can refresh only the ones gone stale when it opens
-- (docs/02_features/04_job_discovery.md AC-009). NULL is a search that has
-- never run.
ALTER TABLE saved_searches ADD COLUMN last_run_at DATETIME;

-- +goose Down
ALTER TABLE saved_searches DROP COLUMN last_run_at;
