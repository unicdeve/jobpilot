-- +goose Up
-- Stage one of ADR-0015: normalize the candidate facts that real
-- application forms ask for.
--
-- This is migration 00004's lesson at larger scale. That one split `name`
-- because profileFactAnswer matched any label containing "name" and so put
-- the full name in a form's separate First and Last fields. The same shape
-- of defect was measured across the rest of the profile, against question
-- labels recorded from live Ashby, Greenhouse and Lever forms:
--
--   contact  -> one string behind both Email and Phone, so a real form's
--               phone field was filled with the candidate's email address
--   links    -> one string behind LinkedIn, Portfolio and Website, so each
--               got every URL, while GitHub and Twitter got nothing
--   work_authorization, salary_preferences
--            -> prose behind Yes/No and numeric questions
--
-- A fact a form asks for separately has to be stored separately. That is
-- the whole decision.

-- One of each: what a person has exactly one of.
ALTER TABLE candidates ADD COLUMN email TEXT NOT NULL DEFAULT '';
ALTER TABLE candidates ADD COLUMN phone TEXT NOT NULL DEFAULT '';

-- Greenhouse asks for Country and Location (City) as separate fields.
ALTER TABLE candidates ADD COLUMN location_city TEXT NOT NULL DEFAULT '';
ALTER TABLE candidates ADD COLUMN location_region TEXT NOT NULL DEFAULT '';
ALTER TABLE candidates ADD COLUMN location_country TEXT NOT NULL DEFAULT '';

-- Availability: "When are you available" (Ashby). Both nullable, because a
-- notice period of zero means "immediately" and is not the same as unsaid.
ALTER TABLE candidates ADD COLUMN notice_period_days INTEGER;
ALTER TABLE candidates ADD COLUMN earliest_start_date DATE;

-- The blobs these replace. Dropped rather than carried: there is no data
-- here worth a compatibility shim, and resumes.markdown_content — the
-- source every one of them was written from — is untouched, so the
-- extract-my-profile action can propose them back (ADR-0015 §7).
--
-- salary_preferences has no replacement on purpose. A salary filter belongs
-- to the saved search, where SearchCriteria already keeps it, and a form's
-- "what are your salary expectations" is a different fact with a different
-- audience — telling an employer a search floor would state a number the
-- user never gave (ADR-0015 §4). It resolves to needs-user-input now.
ALTER TABLE candidates DROP COLUMN contact;
ALTER TABLE candidates DROP COLUMN location;
ALTER TABLE candidates DROP COLUMN links;
ALTER TABLE candidates DROP COLUMN work_authorization;
ALTER TABLE candidates DROP COLUMN salary_preferences;

-- Several of each: what a person has more than one of.

-- One link per kind, because that is what a form asks for. A person with
-- two portfolios picks the one they want employers to see, which is the
-- same choice they would make in the form itself.
CREATE TABLE IF NOT EXISTS candidate_links (
	id TEXT PRIMARY KEY,
	candidate_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	url TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE,
	UNIQUE (candidate_id, kind)
);

-- Work authorization is per country, because the question is: "will you
-- require sponsorship" is always asked about somewhere. country is the
-- country name as the user writes it, matched against a job's locations;
-- there is no country database here and no guessing.
CREATE TABLE IF NOT EXISTS candidate_work_authorizations (
	id TEXT PRIMARY KEY,
	candidate_id TEXT NOT NULL,
	country TEXT NOT NULL,
	is_authorized INTEGER NOT NULL DEFAULT 0,
	requires_sponsorship INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE CASCADE,
	UNIQUE (candidate_id, country)
);

CREATE INDEX IF NOT EXISTS idx_candidate_links_candidate ON candidate_links (candidate_id);
CREATE INDEX IF NOT EXISTS idx_candidate_work_auth_candidate ON candidate_work_authorizations (candidate_id);

-- +goose Down
DROP INDEX IF EXISTS idx_candidate_work_auth_candidate;
DROP INDEX IF EXISTS idx_candidate_links_candidate;
DROP TABLE IF EXISTS candidate_work_authorizations;
DROP TABLE IF EXISTS candidate_links;

ALTER TABLE candidates ADD COLUMN salary_preferences TEXT;
ALTER TABLE candidates ADD COLUMN work_authorization TEXT;
ALTER TABLE candidates ADD COLUMN links TEXT;
ALTER TABLE candidates ADD COLUMN location TEXT;
ALTER TABLE candidates ADD COLUMN contact TEXT;

ALTER TABLE candidates DROP COLUMN earliest_start_date;
ALTER TABLE candidates DROP COLUMN notice_period_days;
ALTER TABLE candidates DROP COLUMN location_country;
ALTER TABLE candidates DROP COLUMN location_region;
ALTER TABLE candidates DROP COLUMN location_city;
ALTER TABLE candidates DROP COLUMN phone;
ALTER TABLE candidates DROP COLUMN email;
