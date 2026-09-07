//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestCandidateRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())
	candidate := &domain.Candidate{Name: "Taiwo", Contact: "taiwo@example.com", Summary: "Go engineer"}

	require.NoError(t, repo.Save(context.Background(), candidate))
	assert.NotEmpty(t, candidate.ID, "Save should generate an ID")
	assert.False(t, candidate.CreatedAt.IsZero())

	got, err := repo.GetByID(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "Taiwo", got.Name)
	assert.Equal(t, "Go engineer", got.Summary)
}

func TestCandidateRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "candidate not found")
}

func TestCandidateRepository_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())
	candidate := &domain.Candidate{Name: "Taiwo"}
	require.NoError(t, repo.Save(context.Background(), candidate))

	candidate.Summary = "Updated summary"
	require.NoError(t, repo.Update(context.Background(), candidate))

	got, err := repo.GetByID(context.Background(), candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated summary", got.Summary)
}

func TestCandidateRepository_Delete(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCandidateRepository(db.Conn())
	candidate := &domain.Candidate{Name: "Taiwo"}
	require.NoError(t, repo.Save(context.Background(), candidate))

	require.NoError(t, repo.Delete(context.Background(), candidate.ID))

	_, err := repo.GetByID(context.Background(), candidate.ID)
	assert.ErrorContains(t, err, "candidate not found")
}

func TestCandidateRepository_GetFirstCandidate(t *testing.T) {
	t.Parallel()

	t.Run("returns the first saved candidate", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewCandidateRepository(db.Conn())
		candidate := &domain.Candidate{Name: "Taiwo"}
		require.NoError(t, repo.Save(context.Background(), candidate))

		got, err := repo.GetFirstCandidate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, candidate.ID, got.ID)
	})

	t.Run("errors when no candidate exists", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		repo := NewCandidateRepository(db.Conn())

		_, err := repo.GetFirstCandidate(context.Background())

		assert.ErrorContains(t, err, "no candidate profile found")
	})
}
