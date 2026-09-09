-- +goose Up

-- profileFactAnswer (internal/core/services/answers.go) matched any
-- question label containing "name", so a real form with separate first/
-- last name fields got the candidate's full name in both. Splitting the
-- column lets that resolver distinguish them properly. Existing full-name
-- values land in first_name with last_name empty rather than being
-- guessed apart.
ALTER TABLE candidates RENAME COLUMN name TO first_name;
ALTER TABLE candidates ADD COLUMN last_name TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE candidates DROP COLUMN last_name;
ALTER TABLE candidates RENAME COLUMN first_name TO name;
