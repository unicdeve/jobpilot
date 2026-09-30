-- +goose Up
-- Where a job can be done from, and whether it offers relocation and visa
-- sponsorship, as ReadSignals reads them from the job's own fields
-- (ADR-0023 §3). All of it is derived, so it starts empty with version 0,
-- and the daemon reads every such job again when it starts.
--
-- workable_countries holds ISO codes with a comma on each side,
-- ",DE,FR,", so a country is one LIKE away without a table of its own:
-- the stored jobs list already scans for its other filters.
ALTER TABLE jobs ADD COLUMN workable_scope TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN workable_places TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN workable_countries TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN workable_note TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN relocation TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN relocation_evidence TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN visa_sponsorship TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN visa_evidence TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN signals_version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_jobs_signals_version ON jobs(signals_version);

-- Where the candidate works from, as an ISO code, and whether a search also
-- keeps jobs elsewhere that help them move (ADR-0023 §4). Empty and false
-- is a search as it was.
ALTER TABLE saved_searches ADD COLUMN work_from TEXT NOT NULL DEFAULT '';
ALTER TABLE saved_searches ADD COLUMN include_relocation BOOLEAN NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE saved_searches DROP COLUMN include_relocation;
ALTER TABLE saved_searches DROP COLUMN work_from;
DROP INDEX idx_jobs_signals_version;
ALTER TABLE jobs DROP COLUMN signals_version;
ALTER TABLE jobs DROP COLUMN visa_evidence;
ALTER TABLE jobs DROP COLUMN visa_sponsorship;
ALTER TABLE jobs DROP COLUMN relocation_evidence;
ALTER TABLE jobs DROP COLUMN relocation;
ALTER TABLE jobs DROP COLUMN workable_note;
ALTER TABLE jobs DROP COLUMN workable_countries;
ALTER TABLE jobs DROP COLUMN workable_places;
ALTER TABLE jobs DROP COLUMN workable_scope;
