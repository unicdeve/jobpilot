-- +goose Up
-- The Agent Activity screen (docs/02_features/12_agent_activity.md) lists
-- every run, and has to name what each one served.

-- A scout run creates several applications and belongs to none of them
-- (ADR-0018 §4), so until now nothing recorded which scout it was. SET
-- NULL rather than CASCADE: deleting a scout should not erase the record
-- of what it did.
ALTER TABLE agent_runs ADD COLUMN scout_id TEXT REFERENCES scouts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_agent_runs_started_at ON agent_runs(started_at);

-- Earlier scout runs named their scout only in the first step's text,
-- written as `scout "<name>", search "<name>"`. Names are unique, so the
-- name finds the scout, unless it has been renamed or deleted since.
UPDATE agent_runs SET scout_id = (
	SELECT s.id FROM scouts s
	JOIN agent_events e ON e.run_id = agent_runs.id AND e.action = 'scout_started'
	WHERE e.input_summary LIKE 'scout "' || s.name || '",%'
	LIMIT 1
) WHERE task_type = 'scout_run';

-- A filled field's step recorded the answer that was typed: an email
-- address, a phone number, a work authorization. The answer is already
-- stored on the application's question, and the trail is shown on its own
-- screen, so the copy is removed and new steps no longer write one.
UPDATE agent_events SET output_summary = NULL
WHERE action = 'fill_field';
UPDATE agent_events SET output_summary = 'the answer is not one of the choices offered'
WHERE action = 'skip_field' AND output_summary LIKE '%is not one of the choices offered';

-- Scout decisions were recorded before policy_result was ever written.
UPDATE agent_events SET policy_result = 'denied'
WHERE action IN ('job_rejected', 'ceiling_reached') AND policy_result IS NULL;
UPDATE agent_events SET policy_result = 'allowed'
WHERE action = 'application_created' AND policy_result IS NULL;

-- +goose Down
-- The answers removed above are not restored: they were never needed here.
DROP INDEX IF EXISTS idx_agent_runs_started_at;
ALTER TABLE agent_runs DROP COLUMN scout_id;
