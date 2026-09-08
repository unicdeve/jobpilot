-- +goose Up

-- agent_runs (migration 00001) had no way to scope a run to the
-- application it was working on, which the browser worker (Phase 6,
-- docs/01_architecture/03_browser_worker.md) and the desktop's per-
-- application activity view both need.
ALTER TABLE agent_runs ADD COLUMN application_id TEXT REFERENCES applications(id);

CREATE INDEX IF NOT EXISTS idx_agent_runs_application_id ON agent_runs(application_id);

-- Distinguishes a question the browser worker extracted from a real
-- application form ('form_extracted') from one entered by hand via
-- `application question add` ('manual'), and field_ref is the browser-
-- fillable reference the worker needs to fill an extracted question's
-- resolved answer back into the page, without re-extracting the form.
ALTER TABLE application_questions ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE application_questions ADD COLUMN field_ref TEXT;

-- +goose Down
ALTER TABLE application_questions DROP COLUMN field_ref;
ALTER TABLE application_questions DROP COLUMN source;

DROP INDEX IF EXISTS idx_agent_runs_application_id;
ALTER TABLE agent_runs DROP COLUMN application_id;
