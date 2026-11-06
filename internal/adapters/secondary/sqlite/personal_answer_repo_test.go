//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestPersonalAnswerRepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	setup := func(t *testing.T) (*DB, *PersonalAnswerRepository) {
		t.Helper()
		db := newTestDB(t)
		execAll(t, db, true, `INSERT INTO candidates (id, first_name) VALUES ('cand', 'Ada')`)
		return db, NewPersonalAnswerRepository(db.Conn())
	}

	t.Run("saves, lists, updates and deletes", func(t *testing.T) {
		t.Parallel()
		_, repo := setup(t)
		values := &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Question: "What do you care about?", Answer: "Honesty"}
		require.NoError(t, repo.Save(ctx, values))
		require.NotEmpty(t, values.ID)

		mine, err := repo.ListByCandidate(ctx, "cand")
		require.NoError(t, err)
		require.Len(t, mine, 1)
		assert.Equal(t, "Honesty", mine[0].Answer)
		assert.Equal(t, "cand", mine[0].CandidateID)

		values.Answer = "Honesty, kindly"
		require.NoError(t, repo.Save(ctx, values))
		got, err := repo.GetByID(ctx, values.ID)
		require.NoError(t, err)
		assert.Equal(t, "Honesty, kindly", got.Answer)
		assert.Equal(t, domain.AnswerValues, got.Key)

		require.NoError(t, repo.Delete(ctx, values.ID))
		_, err = repo.GetByID(ctx, values.ID)
		assert.Error(t, err)
		assert.NoError(t, repo.Delete(ctx, values.ID), "deleting what is gone is not an error")
	})

	t.Run("an answer has an owner", func(t *testing.T) {
		t.Parallel()
		_, repo := setup(t)
		assert.Error(t, repo.Save(ctx, &domain.PersonalAnswer{Key: domain.AnswerValues, Answer: "x"}))
	})

	t.Run("one answer per standing question; kept answers repeat", func(t *testing.T) {
		t.Parallel()
		_, repo := setup(t)
		require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Answer: "a"}))
		assert.Error(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Answer: "b"}))

		for range 2 {
			require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerKept, Question: "Why us?", Answer: "a"}))
		}
	})

	t.Run("answers go with the profile", func(t *testing.T) {
		t.Parallel()
		db, repo := setup(t)
		require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Answer: "a"}))

		execAll(t, db, true, `DELETE FROM candidates WHERE id = 'cand'`)
		assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM personal_answers`))
	})
}

// TestMigration00018_MovesWritingPreferences is ADR-0022 §3: what the
// candidate wrote about tone and length is kept, exactly, in its new home,
// and the column it came from is gone.
func TestMigration00018_MovesWritingPreferences(t *testing.T) {
	t.Parallel()
	db := migratedTo(t, 17)
	execAll(t, db, true,
		`INSERT INTO candidates (id, first_name, application_preferences) VALUES ('said', 'Ada', 'Warm, under 200 words')`,
		`INSERT INTO candidates (id, first_name, application_preferences) VALUES ('blank', 'Bob', '   ')`,
		`INSERT INTO candidates (id, first_name) VALUES ('unsaid', 'Cy')`,
	)

	migrateUpTo(t, db, 18)

	var key, answer string
	require.NoError(t, db.Conn().QueryRow(
		`SELECT key, answer FROM personal_answers WHERE candidate_id = 'said'`).Scan(&key, &answer))
	assert.Equal(t, "writing_preferences", key)
	assert.Equal(t, "Warm, under 200 words", answer)
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM personal_answers`), "nothing is made up for a candidate who said nothing")
	assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM pragma_table_info('candidates') WHERE name = 'application_preferences'`))
}
