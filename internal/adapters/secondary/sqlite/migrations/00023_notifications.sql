-- +goose Up
-- What JobPilot told the user, or will (ADR-0028). A notification is
-- recorded whether or not it can be posted, so one the operating system
-- never showed still reaches the user in the app.
--
-- Its text is counts and a scout's name, written in core; nothing about a
-- job or the candidate is stored here, because the same text shows on a
-- lock screen.
CREATE TABLE IF NOT EXISTS notifications (
	id TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	-- SET NULL rather than CASCADE: a notification is the record of what
	-- the user was told, and outlives the scout and the run it was about.
	scout_id TEXT REFERENCES scouts(id) ON DELETE SET NULL,
	run_id TEXT REFERENCES agent_runs(id) ON DELETE SET NULL,
	created_at DATETIME NOT NULL,
	-- When the operating system was given it. Email, when it comes, is a
	-- column of its own beside this one (ADR-0028 §5).
	posted_at DATETIME,
	-- When the user saw it in JobPilot.
	seen_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_notifications_created_at;
DROP TABLE IF EXISTS notifications;
