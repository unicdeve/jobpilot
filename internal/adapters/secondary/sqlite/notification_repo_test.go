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

func TestNotificationRepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewNotificationRepository(db.Conn())
	search := seedSavedSearch(t, db)
	scout := &domain.Scout{Name: "Mornings", SavedSearchID: search.ID, MinMatchScore: 70,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true}
	require.NoError(t, NewScoutRepository(db.Conn()).Save(ctx, scout))

	base := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
	older := &domain.Notification{Kind: domain.NotificationScoutFound, Title: "Mornings found 2 jobs for you",
		Body: "They’re ready for review.", ScoutID: scout.ID, CreatedAt: base}
	newer := &domain.Notification{Kind: domain.NotificationScoutFailed, Title: "Mornings couldn’t run",
		Body: "Open JobPilot to see why.", CreatedAt: base.Add(time.Minute)}
	require.NoError(t, repo.Save(ctx, older))
	require.NoError(t, repo.Save(ctx, newer))

	all, err := repo.List(ctx, false, 0)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, newer.ID, all[0].ID, "newest first")
	assert.Equal(t, scout.ID, all[1].ScoutID)
	assert.Empty(t, all[0].ScoutID)

	unposted, err := repo.ListUnposted(ctx, base)
	require.NoError(t, err)
	require.Len(t, unposted, 2)
	assert.Equal(t, older.ID, unposted[0].ID, "oldest first, the order they happened")
	unposted, err = repo.ListUnposted(ctx, base.Add(30*time.Second))
	require.NoError(t, err)
	require.Len(t, unposted, 1)

	claimed, err := repo.ClaimPosted(ctx, older.ID, base.Add(time.Hour))
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = repo.ClaimPosted(ctx, older.ID, base.Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, claimed, "only the first claim posts")
	_, err = repo.ClaimPosted(ctx, "missing", base)
	assert.ErrorIs(t, err, ports.ErrNotFound)

	marked, err := repo.MarkSeen(ctx, newer.ID, base.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, marked)
	unseen, err := repo.List(ctx, true, 0)
	require.NoError(t, err)
	require.Len(t, unseen, 1)
	assert.Equal(t, older.ID, unseen[0].ID)
	marked, err = repo.MarkSeen(ctx, "", base.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, marked)
	_, err = repo.MarkSeen(ctx, "missing", base)
	assert.ErrorIs(t, err, ports.ErrNotFound)

	// A notification outlives the scout it was about (ADR-0028 §4).
	require.NoError(t, NewScoutRepository(db.Conn()).Delete(ctx, scout.ID))
	all, err = repo.List(ctx, false, 0)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Empty(t, all[1].ScoutID)
}
