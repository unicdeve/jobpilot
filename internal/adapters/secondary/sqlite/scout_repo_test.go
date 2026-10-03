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

func seedSavedSearch(t *testing.T, db *DB) *domain.SavedSearch {
	t.Helper()
	search := &domain.SavedSearch{Name: "Go roles", Criteria: domain.SearchCriteria{Keywords: "Go"}}
	require.NoError(t, NewSavedSearchRepository(db.Conn()).Save(context.Background(), search))
	return search
}

func TestScoutRepository_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	search := seedSavedSearch(t, db)
	repo := NewScoutRepository(db.Conn())

	scout := &domain.Scout{
		Name: "Daily Go", SavedSearchID: search.ID, MinMatchScore: 75,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceFetch, IsEnabled: true,
	}
	require.NoError(t, repo.Save(ctx, scout))
	require.NotEmpty(t, scout.ID)

	got, err := repo.GetByID(ctx, scout.ID)
	require.NoError(t, err)
	assert.Equal(t, "Daily Go", got.Name)
	assert.Equal(t, search.ID, got.SavedSearchID)
	assert.Equal(t, 75, got.MinMatchScore)
	assert.Equal(t, domain.ScoutSourceFetch, got.Source)
	assert.True(t, got.IsEnabled)
	assert.Nil(t, got.LastRunAt, "a scout that has never run has no last run")

	got.MinMatchScore = 80
	got.IsEnabled = false
	require.NoError(t, repo.Update(ctx, got))

	updated, err := repo.GetByID(ctx, scout.ID)
	require.NoError(t, err)
	assert.Equal(t, 80, updated.MinMatchScore)
	assert.False(t, updated.IsEnabled)

	scouts, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, scouts, 1)

	require.NoError(t, repo.Delete(ctx, scout.ID))
	_, err = repo.GetByID(ctx, scout.ID)
	assert.ErrorContains(t, err, "scout not found")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

// TestScoutRepository_DeletingTheSearchTakesTheScout covers the cascade
// ADR-0018 chose: a scout without its saved search has no criteria at all,
// and one that answered "everything matches" instead would be the worst
// failure available to it.
func TestScoutRepository_DeletingTheSearchTakesTheScout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	search := seedSavedSearch(t, db)
	repo := NewScoutRepository(db.Conn())

	scout := &domain.Scout{
		Name: "Daily Go", SavedSearchID: search.ID, MinMatchScore: 70,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true,
	}
	require.NoError(t, repo.Save(ctx, scout))

	require.NoError(t, NewSavedSearchRepository(db.Conn()).Delete(ctx, search.ID))

	_, err := repo.GetByID(ctx, scout.ID)
	assert.ErrorContains(t, err, "scout not found")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

// TestAgentRunRepository_ScoutRunHasNoApplication covers the fix ADR-0018
// §4 identified: application_id is a foreign key and foreign keys are on,
// so a run that belongs to no application must store NULL rather than the
// empty string, which names an application that does not exist.
func TestAgentRunRepository_ScoutRunHasNoApplication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	repo := NewAgentRunRepository(db.Conn())

	run := &domain.AgentRun{TaskType: "scout_run", Status: domain.AgentRunStatusRunning}
	require.NoError(t, repo.Save(ctx, run), "an empty application_id must store as NULL")

	got, err := repo.GetByID(ctx, run.ID)
	require.NoError(t, err)
	assert.Empty(t, got.ApplicationID)

	unattached, err := repo.ListUnattached(ctx, 10)
	require.NoError(t, err)
	require.Len(t, unattached, 1)
	assert.Equal(t, run.ID, unattached[0].ID)
}

func TestScoutRepository_Schedule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	search := seedSavedSearch(t, db)
	repo := NewScoutRepository(db.Conn())
	set := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)

	scout := &domain.Scout{
		Name: "Mornings", SavedSearchID: search.ID, MinMatchScore: 70,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true,
		Schedule:      domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: 8 * 60, EveryHours: 3},
		ScheduleSetAt: &set,
	}
	require.NoError(t, repo.Save(ctx, scout))

	got, err := repo.GetByID(ctx, scout.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: 480}, got.Schedule,
		"what a schedule's kind does not use is not kept")
	require.NotNil(t, got.ScheduleSetAt)
	assert.True(t, set.Equal(*got.ScheduleSetAt))

	got.Schedule = domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 6}
	require.NoError(t, repo.Update(ctx, got))
	updated, err := repo.GetByID(ctx, scout.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 6}, updated.Schedule)
}

// TestScoutRepository_AnEditIsNotARun covers the fix ADR-0027 needed:
// Update used to write last_run_at from whatever it was given, so saving a
// scout from a form that does not carry it erased its last run.
func TestScoutRepository_AnEditIsNotARun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	search := seedSavedSearch(t, db)
	repo := NewScoutRepository(db.Conn())
	scout := &domain.Scout{
		Name: "Daily Go", SavedSearchID: search.ID, MinMatchScore: 70,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true,
	}
	require.NoError(t, repo.Save(ctx, scout))
	ran := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	require.NoError(t, repo.MarkRun(ctx, scout.ID, ran))

	edit := *scout
	edit.Name, edit.LastRunAt = "Weekday Go", nil
	require.NoError(t, repo.Update(ctx, &edit))

	got, err := repo.GetByID(ctx, scout.ID)
	require.NoError(t, err)
	assert.Equal(t, "Weekday Go", got.Name)
	require.NotNil(t, got.LastRunAt)
	assert.True(t, ran.Equal(*got.LastRunAt))

	assert.ErrorIs(t, repo.MarkRun(ctx, "missing", ran), ports.ErrNotFound)
}

func TestAgentRunRepository_LastScoutStart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	search := seedSavedSearch(t, db)
	scout := &domain.Scout{
		Name: "Daily Go", SavedSearchID: search.ID, MinMatchScore: 70,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true,
	}
	require.NoError(t, NewScoutRepository(db.Conn()).Save(ctx, scout))
	repo := NewAgentRunRepository(db.Conn())

	last, err := repo.LastScoutStart(ctx, scout.ID)
	require.NoError(t, err)
	assert.Nil(t, last, "a scout that never ran has no last start")

	earlier := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	later := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	for _, started := range []time.Time{later, earlier} {
		require.NoError(t, repo.Save(ctx, &domain.AgentRun{
			ScoutID: scout.ID, TaskType: domain.AgentTaskScoutRun,
			Status: domain.AgentRunStatusCompleted, StartedAt: started,
			Trigger: domain.RunTriggerSchedule,
		}))
	}

	last, err = repo.LastScoutStart(ctx, scout.ID)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.True(t, later.Equal(*last))

	runs, err := repo.ListUnattached(ctx, 10)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Equal(t, domain.RunTriggerSchedule, runs[0].Trigger)

	manual := &domain.AgentRun{TaskType: domain.AgentTaskScoutRun, Status: domain.AgentRunStatusRunning}
	require.NoError(t, repo.Save(ctx, manual))
	got, err := repo.GetByID(ctx, manual.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.RunTriggerManual, got.Trigger, "a run says who started it even when nobody said")
}
