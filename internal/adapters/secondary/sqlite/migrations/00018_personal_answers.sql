-- +goose Up
-- What the candidate says about themselves (ADR-0022,
-- docs/02_features/16_about_you.md): answers to the About you questions,
-- a writing sample, writing preferences and kept answers, owned by the
-- candidate; answers for one application, owned by it. Each row goes with
-- its owner.
CREATE TABLE personal_answers (
    id             TEXT PRIMARY KEY,
    candidate_id   TEXT REFERENCES candidates(id)   ON DELETE CASCADE,
    application_id TEXT REFERENCES applications(id) ON DELETE CASCADE,
    key            TEXT NOT NULL,
    question       TEXT NOT NULL DEFAULT '',
    answer         TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    CHECK ((candidate_id IS NULL) != (application_id IS NULL))
);

CREATE INDEX idx_personal_answers_candidate ON personal_answers(candidate_id);
CREATE INDEX idx_personal_answers_application ON personal_answers(application_id);

-- One answer per question: every key a candidate has but "kept", and an
-- application's "draws_you". An application's proposed questions are
-- capped by core, their only writer.
CREATE UNIQUE INDEX idx_personal_answers_candidate_key
    ON personal_answers(candidate_id, key)
    WHERE candidate_id IS NOT NULL AND key <> 'kept';
CREATE UNIQUE INDEX idx_personal_answers_application_key
    ON personal_answers(application_id, key)
    WHERE application_id IS NOT NULL AND key = 'draws_you';

-- Writing preferences move here from the candidate (ADR-0022 §3), exactly
-- as they were.
INSERT INTO personal_answers (id, candidate_id, key, question, answer, created_at, updated_at)
SELECT lower(hex(randomblob(16))), id, 'writing_preferences', '', application_preferences,
       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM candidates
WHERE trim(coalesce(application_preferences, '')) <> '';

ALTER TABLE candidates DROP COLUMN application_preferences;

-- +goose Down
ALTER TABLE candidates ADD COLUMN application_preferences TEXT;

UPDATE candidates SET application_preferences = (
    SELECT answer FROM personal_answers
    WHERE personal_answers.candidate_id = candidates.id AND key = 'writing_preferences'
);

DROP TABLE personal_answers;
