//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestCoverLetterRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	repo := NewCoverLetterRepository(db.Conn())

	cl := &domain.CoverLetter{ApplicationID: app.ID, Content: "Dear hiring manager...", ModelProvider: "ollama", ModelName: "llama3"}
	require.NoError(t, repo.Save(context.Background(), cl))
	assert.NotEmpty(t, cl.ID)

	got, err := repo.GetByID(context.Background(), cl.ID)
	require.NoError(t, err)
	assert.Equal(t, "Dear hiring manager...", got.Content)
	assert.False(t, got.UserEdited)
}

func TestCoverLetterRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewCoverLetterRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "cover letter not found")
}

func TestCoverLetterRepository_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	repo := NewCoverLetterRepository(db.Conn())
	cl := &domain.CoverLetter{ApplicationID: app.ID, Content: "original draft"}
	require.NoError(t, repo.Save(context.Background(), cl))

	cl.Content = "edited by user"
	cl.UserEdited = true
	require.NoError(t, repo.Update(context.Background(), cl))

	got, err := repo.GetByID(context.Background(), cl.ID)
	require.NoError(t, err)
	assert.Equal(t, "edited by user", got.Content)
	assert.True(t, got.UserEdited)
}

func TestCoverLetterRepository_ListByApplication(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	otherApp := saveTestApplication(t, db, candidate.ID, saveTestJob(t, db, "https://acme.example/job-2").ID)
	repo := NewCoverLetterRepository(db.Conn())

	first := &domain.CoverLetter{ApplicationID: app.ID, Content: "v1"}
	second := &domain.CoverLetter{ApplicationID: app.ID, Content: "v2 (regenerated)"}
	other := &domain.CoverLetter{ApplicationID: otherApp.ID, Content: "unrelated"}
	require.NoError(t, repo.Save(context.Background(), first))
	require.NoError(t, repo.Save(context.Background(), second))
	require.NoError(t, repo.Save(context.Background(), other))

	versions, err := repo.ListByApplication(context.Background(), app.ID)

	require.NoError(t, err)
	require.Len(t, versions, 2, "regeneration must preserve prior versions rather than overwrite them")
	assert.Equal(t, "v1", versions[0].Content)
	assert.Equal(t, "v2 (regenerated)", versions[1].Content)
}
