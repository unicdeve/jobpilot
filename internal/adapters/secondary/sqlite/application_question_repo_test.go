//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func TestApplicationQuestionRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	repo := NewApplicationQuestionRepository(db.Conn())

	q := &domain.ApplicationQuestion{
		ApplicationID: app.ID,
		Label:         "What is your email address?",
		Type:          domain.QuestionTypeFactual,
		Required:      true,
		Status:        domain.QuestionStatusPending,
	}
	require.NoError(t, repo.Save(context.Background(), q))
	assert.NotEmpty(t, q.ID)

	got, err := repo.GetByID(context.Background(), q.ID)
	require.NoError(t, err)
	assert.Equal(t, "What is your email address?", got.Label)
	assert.Equal(t, domain.QuestionTypeFactual, got.Type)
	assert.True(t, got.Required)
	assert.Equal(t, domain.QuestionStatusPending, got.Status)
	assert.Empty(t, got.Category, "unresolved question should round-trip with no category set")
}

func TestApplicationQuestionRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewApplicationQuestionRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "application question not found")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestApplicationQuestionRepository_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	repo := NewApplicationQuestionRepository(db.Conn())
	q := &domain.ApplicationQuestion{ApplicationID: app.ID, Label: "Location?", Type: domain.QuestionTypeFactual}
	require.NoError(t, repo.Save(context.Background(), q))

	q.Answer = "Remote"
	q.AnswerSource = "candidate.location"
	q.Category = domain.AnswerCategoryProfileFact
	q.Confidence = 1.0
	q.Status = domain.QuestionStatusAnswered
	require.NoError(t, repo.Update(context.Background(), q))

	got, err := repo.GetByID(context.Background(), q.ID)
	require.NoError(t, err)
	assert.Equal(t, "Remote", got.Answer)
	assert.Equal(t, domain.AnswerCategoryProfileFact, got.Category)
	assert.Equal(t, 1.0, got.Confidence)
	assert.Equal(t, domain.QuestionStatusAnswered, got.Status)
}

func TestApplicationQuestionRepository_ListByApplication(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	otherApp := saveTestApplication(t, db, candidate.ID, saveTestJob(t, db, "https://acme.example/job-2").ID)
	repo := NewApplicationQuestionRepository(db.Conn())

	q1 := &domain.ApplicationQuestion{ApplicationID: app.ID, Label: "Q1"}
	q2 := &domain.ApplicationQuestion{ApplicationID: app.ID, Label: "Q2"}
	other := &domain.ApplicationQuestion{ApplicationID: otherApp.ID, Label: "unrelated"}
	require.NoError(t, repo.Save(context.Background(), q1))
	require.NoError(t, repo.Save(context.Background(), q2))
	require.NoError(t, repo.Save(context.Background(), other))

	questions, err := repo.ListByApplication(context.Background(), app.ID)

	require.NoError(t, err)
	assert.Len(t, questions, 2)
}
