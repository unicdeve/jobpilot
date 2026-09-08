//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestAgentEventRepository_SaveAndListByRun(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	runRepo := NewAgentRunRepository(db.Conn())
	run := &domain.AgentRun{ApplicationID: app.ID, TaskType: "prepare_application", Status: domain.AgentRunStatusRunning}
	require.NoError(t, runRepo.Save(context.Background(), run))

	otherRun := &domain.AgentRun{ApplicationID: app.ID, TaskType: "prepare_application", Status: domain.AgentRunStatusRunning}
	require.NoError(t, runRepo.Save(context.Background(), otherRun))

	repo := NewAgentEventRepository(db.Conn())
	navigate := &domain.AgentEvent{RunID: run.ID, Action: "navigate", Tool: "browser", Status: "ok", OutputSummary: "https://acme.example/job-1", DurationMs: 120}
	extract := &domain.AgentEvent{RunID: run.ID, Action: "extract_form", Tool: "browser", Status: "ok", OutputSummary: "2 fields found"}
	unrelated := &domain.AgentEvent{RunID: otherRun.ID, Action: "navigate", Tool: "browser", Status: "ok"}
	require.NoError(t, repo.Save(context.Background(), navigate))
	require.NoError(t, repo.Save(context.Background(), extract))
	require.NoError(t, repo.Save(context.Background(), unrelated))

	events, err := repo.ListByRun(context.Background(), run.ID)

	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "navigate", events[0].Action)
	assert.Equal(t, "https://acme.example/job-1", events[0].OutputSummary)
	assert.Equal(t, int64(120), events[0].DurationMs)
	assert.Equal(t, "extract_form", events[1].Action)
}
