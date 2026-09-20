-- +goose Up
-- Autopilot is no longer an application mode
-- (docs/09_decisions/0014-scouts-and-no-unattended-submission.md).
--
-- The mode was never acted on — no code has ever read it to decide
-- behaviour — but applications.mode is free TEXT with no constraint and the
-- service stored whatever it was handed, so `application create --mode
-- autopilot` really did persist that value. Any such row is rewritten to
-- review, which is not a lossy choice: review is what those applications
-- have actually been getting all along.
--
-- The capability did not disappear with the mode. Unattended discovery and
-- matching moved to a Scout, which prepares applications for the user to
-- review and submits nothing.
UPDATE applications SET mode = 'review' WHERE mode = 'autopilot';

-- +goose Down
-- Deliberately irreversible. Which applications were created as 'autopilot'
-- is not recorded anywhere else, and restoring a mode the code no longer
-- has would put back a value nothing can read.
SELECT 1;
