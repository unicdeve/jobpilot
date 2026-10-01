-- +goose Up
-- The working hours a job asks for, as ReadSignals reads them (ADR-0025):
-- required or preferred, a window of UTC offsets, and the sentence it was
-- read from. Derived like the other signals, so it starts empty, and the
-- daemon reads every job again because SignalsVersion rose with it.
ALTER TABLE jobs ADD COLUMN hours_status TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN hours_from REAL NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN hours_to REAL NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN hours_evidence TEXT NOT NULL DEFAULT '';

-- The IANA time zone the candidate works in, or empty.
ALTER TABLE candidates ADD COLUMN time_zone TEXT NOT NULL DEFAULT '';

-- Whether a search leaves out jobs whose required hours the candidate
-- cannot work, and how many hours they would move their day for them.
ALTER TABLE saved_searches ADD COLUMN fit_hours BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE saved_searches ADD COLUMN hours_shift INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE saved_searches DROP COLUMN hours_shift;
ALTER TABLE saved_searches DROP COLUMN fit_hours;
ALTER TABLE candidates DROP COLUMN time_zone;
ALTER TABLE jobs DROP COLUMN hours_evidence;
ALTER TABLE jobs DROP COLUMN hours_to;
ALTER TABLE jobs DROP COLUMN hours_from;
ALTER TABLE jobs DROP COLUMN hours_status;
