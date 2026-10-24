-- +goose Up
-- Where the user said an application's form is, when the job's own link
-- does not get there: a posting behind a check that refuses JobPilot's
-- browser, say (ADR-0033 §4). Empty means the job's own link.
ALTER TABLE applications ADD COLUMN apply_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE applications DROP COLUMN apply_url;
