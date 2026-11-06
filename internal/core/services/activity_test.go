package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func TestAgentActivityService_ListRuns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runs := newFakeAgentRunRepository()
	start := time.Now().Add(-time.Hour)
	for i, status := range []domain.AgentRunStatus{domain.AgentRunStatusFailed, domain.AgentRunStatusCompleted, domain.AgentRunStatusCompleted} {
		require.NoError(t, runs.Save(ctx, &domain.AgentRun{
			TaskType: domain.AgentTaskScoutRun, Status: status,
			StartedAt: start.Add(time.Duration(i) * time.Minute),
		}))
	}
	svc := NewAgentActivityService(runs, newFakeAgentEventRepository())

	t.Run("lists runs newest first, with a default page size", func(t *testing.T) {
		page, err := svc.ListRuns(ctx, domain.AgentRunFilter{})
		require.NoError(t, err)
		assert.Equal(t, 3, page.Total)
		require.Len(t, page.Runs, 3)
		assert.True(t, page.Runs[0].StartedAt.After(page.Runs[2].StartedAt))
	})

	t.Run("filters by kind and status", func(t *testing.T) {
		page, err := svc.ListRuns(ctx, domain.AgentRunFilter{TaskType: domain.AgentTaskScoutRun})
		require.NoError(t, err)
		assert.Equal(t, 3, page.Total)

		page, err = svc.ListRuns(ctx, domain.AgentRunFilter{Status: domain.AgentRunStatusFailed})
		require.NoError(t, err)
		assert.Equal(t, 1, page.Total)
	})

	t.Run("an oversized page is capped rather than refused", func(t *testing.T) {
		page, err := svc.ListRuns(ctx, domain.AgentRunFilter{Limit: 10_000})
		require.NoError(t, err)
		assert.Len(t, page.Runs, 3)
	})

	for name, filter := range map[string]domain.AgentRunFilter{
		"an unknown kind":   {TaskType: "prepare_application"},
		"an unknown status": {Status: "waiting_for_user"},
		"a negative offset": {Offset: -1},
	} {
		t.Run(name+" is refused, not answered with nothing", func(t *testing.T) {
			_, err := svc.ListRuns(ctx, filter)
			assert.ErrorIs(t, err, ErrInvalidActivityFilter)
		})
	}
}

func TestAgentActivityService_GetRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runs := newFakeAgentRunRepository()
	events := newFakeAgentEventRepository()
	run := &domain.AgentRun{TaskType: domain.AgentTaskScoutRun, Status: domain.AgentRunStatusCompleted}
	require.NoError(t, runs.Save(ctx, run))
	require.NoError(t, events.Save(ctx, &domain.AgentEvent{RunID: run.ID, Action: "scout_started"}))
	require.NoError(t, events.Save(ctx, &domain.AgentEvent{RunID: run.ID, Action: "scout_finished"}))
	svc := NewAgentActivityService(runs, events)

	detail, err := svc.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run.ID, detail.Run.ID)
	require.Len(t, detail.Events, 2)
	assert.Equal(t, "scout_started", detail.Events[0].Action)

	_, err = svc.GetRun(ctx, "does-not-exist")
	assert.ErrorIs(t, err, ports.ErrAgentRunNotFound)
}
