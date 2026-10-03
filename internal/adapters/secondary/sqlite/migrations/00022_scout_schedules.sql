-- +goose Up
-- When a Scout runs on its own (ADR-0027). Two shapes rather than a cron
-- string: every N hours, or daily at a local time, kept in typed columns
-- the schema and the compiler can check. Every existing scout is off, so
-- nothing starts running by itself because of an upgrade.
ALTER TABLE scouts ADD COLUMN schedule TEXT NOT NULL DEFAULT 'off';
ALTER TABLE scouts ADD COLUMN every_hours INTEGER NOT NULL DEFAULT 0;
-- Minutes after the computer's local midnight.
ALTER TABLE scouts ADD COLUMN daily_at INTEGER NOT NULL DEFAULT 0;
-- When the schedule last changed. A scout's first scheduled run is
-- measured from it, so one scheduled at 10:00 for "daily at 08:00" waits
-- for tomorrow rather than running the moment it is saved.
ALTER TABLE scouts ADD COLUMN schedule_set_at DATETIME;

-- What started a run: the user, or a scout's schedule. Every run so far
-- was started by the user.
ALTER TABLE agent_runs ADD COLUMN started_by TEXT NOT NULL DEFAULT 'manual';

-- Whether a scout is due is measured from its last run's start.
CREATE INDEX IF NOT EXISTS idx_agent_runs_scout_started ON agent_runs(scout_id, started_at);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_runs_scout_started;
ALTER TABLE agent_runs DROP COLUMN started_by;
ALTER TABLE scouts DROP COLUMN schedule_set_at;
ALTER TABLE scouts DROP COLUMN daily_at;
ALTER TABLE scouts DROP COLUMN every_hours;
ALTER TABLE scouts DROP COLUMN schedule;
