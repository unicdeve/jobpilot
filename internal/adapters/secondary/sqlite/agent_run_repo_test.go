//go:build integration

package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func TestAgentRunRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewAgentRunRepository(db.Conn())

	run := &domain.AgentRun{TaskType: domain.AgentTaskScoutRun, Status: domain.AgentRunStatusRunning, Trigger: domain.RunTriggerSchedule}
	require.NoError(t, repo.Save(context.Background(), run))
	assert.NotEmpty(t, run.ID)

	got, err := repo.GetByID(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentTaskScoutRun, got.TaskType)
	assert.Equal(t, domain.AgentRunStatusRunning, got.Status)
	assert.Equal(t, domain.RunTriggerSchedule, got.Trigger)
	assert.False(t, got.StartedAt.IsZero())
	assert.Nil(t, got.CompletedAt)
}

func TestAgentRunRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewAgentRunRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "agent run not found")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestAgentRunRepository_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewAgentRunRepository(db.Conn())

	run := &domain.AgentRun{TaskType: domain.AgentTaskScoutRun, Status: domain.AgentRunStatusRunning}
	require.NoError(t, repo.Save(context.Background(), run))

	completedAt := time.Now()
	run.Status = domain.AgentRunStatusFailed
	run.Error = "no source could be reached"
	run.CompletedAt = &completedAt
	require.NoError(t, repo.Update(context.Background(), run))

	got, err := repo.GetByID(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusFailed, got.Status)
	assert.Equal(t, "no source could be reached", got.Error)
	require.NotNil(t, got.CompletedAt)
}

func TestAgentRunRepository_CloseUnfinished(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewAgentRunRepository(db.Conn())
	ctx := context.Background()

	byStatus := map[domain.AgentRunStatus]*domain.AgentRun{}
	for _, status := range []domain.AgentRunStatus{
		domain.AgentRunStatusRunning, domain.AgentRunStatusCompleted, domain.AgentRunStatusFailed,
	} {
		run := &domain.AgentRun{TaskType: domain.AgentTaskScoutRun, Status: status, Error: "earlier"}
		require.NoError(t, repo.Save(ctx, run))
		byStatus[status] = run
	}

	closed, err := repo.CloseUnfinished(ctx, "JobPilot stopped while this run was in progress", time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, closed)

	got, err := repo.GetByID(ctx, byStatus[domain.AgentRunStatusRunning].ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentRunStatusFailed, got.Status, "a running run is closed")
	assert.Equal(t, "JobPilot stopped while this run was in progress", got.Error)
	assert.NotNil(t, got.CompletedAt)

	for _, status := range []domain.AgentRunStatus{domain.AgentRunStatusCompleted, domain.AgentRunStatusFailed} {
		got, err := repo.GetByID(ctx, byStatus[status].ID)
		require.NoError(t, err)
		assert.Equal(t, status, got.Status, "a finished run keeps how it finished")
		assert.Equal(t, "earlier", got.Error)
	}
}
