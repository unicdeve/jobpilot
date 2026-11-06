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
	runRepo := NewAgentRunRepository(db.Conn())
	run := &domain.AgentRun{TaskType: domain.AgentTaskScoutRun, Status: domain.AgentRunStatusRunning}
	require.NoError(t, runRepo.Save(context.Background(), run))

	otherRun := &domain.AgentRun{TaskType: domain.AgentTaskScoutRun, Status: domain.AgentRunStatusRunning}
	require.NoError(t, runRepo.Save(context.Background(), otherRun))

	repo := NewAgentEventRepository(db.Conn())
	navigate := &domain.AgentEvent{RunID: run.ID, Action: "scout_started", Tool: "scout", Status: "ok", OutputSummary: "Daily Go", DurationMs: 120}
	extract := &domain.AgentEvent{RunID: run.ID, Action: "jobs_collected", Tool: "scout", Status: "ok", OutputSummary: "2 jobs found"}
	unrelated := &domain.AgentEvent{RunID: otherRun.ID, Action: "scout_started", Tool: "scout", Status: "ok"}
	require.NoError(t, repo.Save(context.Background(), navigate))
	require.NoError(t, repo.Save(context.Background(), extract))
	require.NoError(t, repo.Save(context.Background(), unrelated))

	events, err := repo.ListByRun(context.Background(), run.ID)

	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "scout_started", events[0].Action)
	assert.Equal(t, "Daily Go", events[0].OutputSummary)
	assert.Equal(t, int64(120), events[0].DurationMs)
	assert.Equal(t, "jobs_collected", events[1].Action)
}
