//go:build integration

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func saveTestJob(t *testing.T, db *DB, canonicalURL string) *domain.Job {
	t.Helper()
	job := &domain.Job{CanonicalURL: canonicalURL}
	require.NoError(t, NewJobRepository(db.Conn()).Save(context.Background(), job))
	return job
}

func TestMatchRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	repo := NewMatchRepository(db.Conn())
	match := &domain.JobMatch{
		JobID:         job.ID,
		CandidateID:   candidate.ID,
		Score:         80,
		Strengths:     []string{"go"},
		MatchedSkills: []string{"go", "sql"},
	}

	require.NoError(t, repo.Save(context.Background(), match))
	assert.NotEmpty(t, match.ID)

	got, err := repo.GetByID(context.Background(), match.ID)
	require.NoError(t, err)
	assert.Equal(t, 80, got.Score)
	assert.Equal(t, []string{"go"}, got.Strengths)
	assert.Equal(t, []string{"go", "sql"}, got.MatchedSkills)
}

func TestMatchRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewMatchRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "match not found")
}

func TestMatchRepository_GetByJobID(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		candidate := saveTestCandidate(t, db)
		job := saveTestJob(t, db, "https://acme.example/job-1")
		repo := NewMatchRepository(db.Conn())
		match := &domain.JobMatch{JobID: job.ID, CandidateID: candidate.ID}
		require.NoError(t, repo.Save(context.Background(), match))

		got, err := repo.GetByJobID(context.Background(), job.ID)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, match.ID, got.ID)
	})

	t.Run("not found returns nil, nil", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewMatchRepository(db.Conn())

		got, err := repo.GetByJobID(context.Background(), "no-such-job")

		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestMatchRepository_ListByCandidate(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate1 := saveTestCandidate(t, db)
	candidate2 := saveTestCandidate(t, db)
	job1 := saveTestJob(t, db, "https://acme.example/job-1")
	job2 := saveTestJob(t, db, "https://acme.example/job-2")
	job3 := saveTestJob(t, db, "https://acme.example/job-3")
	repo := NewMatchRepository(db.Conn())
	older := &domain.JobMatch{JobID: job1.ID, CandidateID: candidate1.ID, CreatedAt: time.Now().Add(-time.Hour)}
	newer := &domain.JobMatch{JobID: job2.ID, CandidateID: candidate1.ID, CreatedAt: time.Now()}
	other := &domain.JobMatch{JobID: job3.ID, CandidateID: candidate2.ID}
	require.NoError(t, repo.Save(context.Background(), older))
	require.NoError(t, repo.Save(context.Background(), newer))
	require.NoError(t, repo.Save(context.Background(), other))

	matches, err := repo.ListByCandidate(context.Background(), candidate1.ID)

	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, newer.ID, matches[0].ID, "most recent match should come first")
	assert.Equal(t, older.ID, matches[1].ID)
}

func TestMatchRepository_Delete(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	repo := NewMatchRepository(db.Conn())
	match := &domain.JobMatch{JobID: job.ID, CandidateID: candidate.ID}
	require.NoError(t, repo.Save(context.Background(), match))

	require.NoError(t, repo.Delete(context.Background(), match.ID))

	_, err := repo.GetByID(context.Background(), match.ID)
	assert.ErrorContains(t, err, "match not found")
}
