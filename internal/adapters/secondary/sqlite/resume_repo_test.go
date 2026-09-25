//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func saveTestCandidate(t *testing.T, db *DB) *domain.Candidate {
	t.Helper()
	candidate := &domain.Candidate{FirstName: "Taiwo"}
	require.NoError(t, NewCandidateRepository(db.Conn()).Save(context.Background(), candidate))
	return candidate
}

func TestResumeRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	repo := NewResumeRepository(db.Conn())
	resume := &domain.Resume{CandidateID: candidate.ID, Name: "resume.pdf", MarkdownContent: "# Resume"}

	require.NoError(t, repo.Save(context.Background(), resume))
	assert.NotEmpty(t, resume.ID)

	got, err := repo.GetByID(context.Background(), resume.ID)
	require.NoError(t, err)
	assert.Equal(t, "resume.pdf", got.Name)
	assert.Equal(t, "# Resume", got.MarkdownContent)
}

func TestResumeRepository_Save_ForeignKeyConstraint(t *testing.T) {
	t.Parallel()

	// Regression test for the bug fixed in 3471f4c: saving a resume whose
	// CandidateID does not reference an existing candidate must fail with a
	// clear FOREIGN KEY error, not silently succeed.
	db := newTestDB(t)
	repo := NewResumeRepository(db.Conn())
	resume := &domain.Resume{CandidateID: "no-such-candidate", Name: "resume.pdf"}

	err := repo.Save(context.Background(), resume)

	require.Error(t, err)
	assert.ErrorContains(t, err, "FOREIGN KEY")
}

func TestResumeRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewResumeRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "resume not found")
}

func TestResumeRepository_GetByCandidateID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	repo := NewResumeRepository(db.Conn())
	require.NoError(t, repo.Save(context.Background(), &domain.Resume{CandidateID: candidate.ID, Name: "a.pdf"}))
	require.NoError(t, repo.Save(context.Background(), &domain.Resume{CandidateID: candidate.ID, Name: "b.pdf"}))

	resumes, err := repo.GetByCandidateID(context.Background(), candidate.ID)

	require.NoError(t, err)
	assert.Len(t, resumes, 2)
}
