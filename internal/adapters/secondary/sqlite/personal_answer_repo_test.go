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
		execAll(t, db, true,
			`INSERT INTO candidates (id, first_name) VALUES ('cand', 'Ada')`,
			`INSERT INTO jobs (id, canonical_url, title) VALUES ('job', 'https://example.com/1', 'Engineer')`,
			`INSERT INTO applications (id, job_id, candidate_id, state) VALUES ('app', 'job', 'cand', 'MATCHED')`,
		)
		return db, NewPersonalAnswerRepository(db.Conn())
	}

	t.Run("saves, lists, updates and deletes", func(t *testing.T) {
		t.Parallel()
		_, repo := setup(t)
		values := &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Question: "What do you care about?", Answer: "Honesty"}
		require.NoError(t, repo.Save(ctx, values))
		require.NotEmpty(t, values.ID)
		draws := &domain.PersonalAnswer{ApplicationID: "app", Key: domain.AnswerDrawsYou, Question: domain.DrawsYouQuestion, Answer: "Tooling"}
		require.NoError(t, repo.Save(ctx, draws))

		mine, err := repo.ListByCandidate(ctx, "cand")
		require.NoError(t, err)
		require.Len(t, mine, 1, "an application's answers are not the candidate's")
		assert.Equal(t, "Honesty", mine[0].Answer)
		assert.Empty(t, mine[0].ApplicationID)
		forApp, err := repo.ListByApplication(ctx, "app")
		require.NoError(t, err)
		require.Len(t, forApp, 1)
		assert.Empty(t, forApp[0].CandidateID)

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

	t.Run("an answer has exactly one owner", func(t *testing.T) {
		t.Parallel()
		_, repo := setup(t)
		assert.Error(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", ApplicationID: "app", Key: domain.AnswerValues, Answer: "x"}))
		assert.Error(t, repo.Save(ctx, &domain.PersonalAnswer{Key: domain.AnswerValues, Answer: "x"}))
	})

	t.Run("one answer per standing question and per fixed question; kept and proposed repeat", func(t *testing.T) {
		t.Parallel()
		_, repo := setup(t)
		require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Answer: "a"}))
		assert.Error(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Answer: "b"}))
		require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{ApplicationID: "app", Key: domain.AnswerDrawsYou, Answer: "a"}))
		assert.Error(t, repo.Save(ctx, &domain.PersonalAnswer{ApplicationID: "app", Key: domain.AnswerDrawsYou, Answer: "b"}))

		for i := 0; i < 2; i++ {
			require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerKept, Question: "Why us?", Answer: "a"}))
			require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{ApplicationID: "app", Key: domain.AnswerProposed, Question: "Tooling?"}))
		}
	})

	t.Run("answers go with their owner", func(t *testing.T) {
		t.Parallel()
		db, repo := setup(t)
		require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{CandidateID: "cand", Key: domain.AnswerValues, Answer: "a"}))
		require.NoError(t, repo.Save(ctx, &domain.PersonalAnswer{ApplicationID: "app", Key: domain.AnswerDrawsYou, Answer: "b"}))

		execAll(t, db, true, `DELETE FROM applications WHERE id = 'app'`)
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM personal_answers`), "an application's answers go with it")
		execAll(t, db, true, `DELETE FROM candidates WHERE id = 'cand'`)
		assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM personal_answers`), "the candidate's go with the profile")
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

	migrateUp(t, db)

	rows, err := NewPersonalAnswerRepository(db.Conn()).ListByCandidate(context.Background(), "said")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, domain.AnswerWritingPreferences, rows[0].Key)
	assert.Equal(t, "Warm, under 200 words", rows[0].Answer)
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM personal_answers`), "nothing is made up for a candidate who said nothing")
	assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM pragma_table_info('candidates') WHERE name = 'application_preferences'`))
}
