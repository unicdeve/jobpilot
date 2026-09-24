-- +goose Up
-- ADR-0019 §6: which credentials exist, and nothing else.
--
-- This table holds no secret. The value lives in the OS keychain, and the
-- only reason this exists is that a keychain cannot be enumerated:
-- go-keyring offers Set, Get, Delete and DeleteAll, and a user-chosen label
-- cannot be guessed, so without a directory the product could not answer
-- "what have I configured?".
--
-- There is no value column. A migration that adds one is a bug, not a
-- feature: it would move a secret into the application database, which
-- AGENTS.md forbids and docs/01_architecture/08_security.md decides
-- against.
--
-- The keychain is the truth and this is a directory. Writes go to the
-- keychain first, so a crash between the two leaves a stored secret that
-- nothing lists — recoverable by setting it again — rather than a listed
-- credential that no connector can read, which looks like corruption and
-- reads as configured in the UI.
CREATE TABLE IF NOT EXISTS credential_names (
	-- Owned by the code that needs the credential, e.g. 'google-search'.
	source TEXT NOT NULL,
	-- The user's, so one source can hold more than one key.
	label TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (source, label)
);

-- +goose Down
DROP TABLE IF EXISTS credential_names;
