-- +goose Up
-- Closing out ADR-0015: the last prose column that earns its place becomes
-- structured, and the ones that do not are removed.
--
-- Normalization worked from the defect table and walked past three columns
-- that no user interface has ever written:
--
--   work_preferences  read by exactly one thing — a substring check for
--                     "remote" in derivedAnswer. Neither the desktop nor
--                     the CLI offers a field for it, so that rule has
--                     never fired for anybody and could not.
--   education         read by nothing at all
--   certifications    read by nothing at all
--
-- remote_preference replaces the first, using the same vocabulary as
-- jobs.remote_type ('remote', 'hybrid', 'onsite') so a preference can be
-- compared with a posting rather than searched for a word inside a
-- paragraph. It is what makes that derivation reachable for the first time.
--
-- The other two are dropped. No recorded form asks about a degree or a
-- certification, nothing reads them, and nothing writes them: they are
-- schema from the Phase 0 design doc that the implementation never grew
-- into. Keeping them would be storing data for a consumer that does not
-- exist, which is what principle 10 rules out.
ALTER TABLE candidates ADD COLUMN remote_preference TEXT NOT NULL DEFAULT '';

ALTER TABLE candidates DROP COLUMN work_preferences;
ALTER TABLE candidates DROP COLUMN education;
ALTER TABLE candidates DROP COLUMN certifications;

-- +goose Down
ALTER TABLE candidates ADD COLUMN certifications TEXT;
ALTER TABLE candidates ADD COLUMN education TEXT;
ALTER TABLE candidates ADD COLUMN work_preferences TEXT;

ALTER TABLE candidates DROP COLUMN remote_preference;
