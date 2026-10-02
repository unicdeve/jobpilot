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
