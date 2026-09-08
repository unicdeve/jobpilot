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

func TestAgentRunRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	repo := NewAgentRunRepository(db.Conn())

	run := &domain.AgentRun{ApplicationID: app.ID, TaskType: "prepare_application", Status: domain.AgentRunStatusRunning}
	require.NoError(t, repo.Save(context.Background(), run))
	assert.NotEmpty(t, run.ID)

	got, err := repo.GetByID(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, app.ID, got.ApplicationID)
	assert.Equal(t, "prepare_application", got.TaskType)
	assert.Equal(t, domain.AgentRunStatusRunning, got.Status)
	assert.False(t, got.StartedAt.IsZero())
	assert.Nil(t, got.CompletedAt)
}

func TestAgentRunRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewAgentRunRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "agent run not found")
}

func TestAgentRunRepository_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	repo := NewAgentRunRepository(db.Conn())

	run := &domain.AgentRun{ApplicationID: app.ID, TaskType: "prepare_application", Status: domain.AgentRunStatusRunning}
	require.NoError(t, repo.Save(context.Background(), run))

	completedAt := time.Now()
	run.Status = domain.AgentRunStatusBlocked
	run.Error = "a captcha challenge is present on the page"
	run.CompletedAt = &completedAt
	require.NoError(t, repo.Update(context.Background(), run))

	got, err := repo.GetByID(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusBlocked, got.Status)
	assert.Equal(t, "a captcha challenge is present on the page", got.Error)
	require.NotNil(t, got.CompletedAt)
}

func TestAgentRunRepository_ListByApplication(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	otherApp := saveTestApplication(t, db, candidate.ID, saveTestJob(t, db, "https://acme.example/job-2").ID)
	repo := NewAgentRunRepository(db.Conn())

	first := &domain.AgentRun{ApplicationID: app.ID, TaskType: "prepare_application", Status: domain.AgentRunStatusCompleted}
	second := &domain.AgentRun{ApplicationID: app.ID, TaskType: "prepare_application", Status: domain.AgentRunStatusBlocked}
	other := &domain.AgentRun{ApplicationID: otherApp.ID, TaskType: "prepare_application", Status: domain.AgentRunStatusCompleted}
	require.NoError(t, repo.Save(context.Background(), first))
	require.NoError(t, repo.Save(context.Background(), second))
	require.NoError(t, repo.Save(context.Background(), other))

	runs, err := repo.ListByApplication(context.Background(), app.ID)

	require.NoError(t, err)
	assert.Len(t, runs, 2)
}
