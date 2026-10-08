package alarms

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

type recordingAlarms struct {
	entries []domain.ScoutAlarmEntry
	written []domain.ScoutAlarm
	removed []string
}

func (r *recordingAlarms) List(context.Context) ([]domain.ScoutAlarmEntry, error) {
	return r.entries, nil
}

func (r *recordingAlarms) Write(_ context.Context, alarm domain.ScoutAlarm) error {
	r.written = append(r.written, alarm)
	return nil
}

func (r *recordingAlarms) Remove(_ context.Context, scoutID string) error {
	r.removed = append(r.removed, scoutID)
	return nil
}

func TestScoped_IsolatesWorkspaceAlarmNames(t *testing.T) {
	base := &recordingAlarms{entries: []domain.ScoutAlarmEntry{
		{ScoutID: "legacy-scout", Fingerprint: "legacy"},
		{ScoutID: "workspace-a.scout-1", Fingerprint: "a"},
		{ScoutID: "workspace-b.scout-1", Fingerprint: "b"},
	}}
	scheduler := Scoped(base, "workspace-a")

	entries, err := scheduler.List(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, domain.ScoutAlarmEntry{ScoutID: "scout-1", Fingerprint: "a"}, entries[0])

	require.NoError(t, scheduler.Write(context.Background(), domain.ScoutAlarm{ScoutID: "scout-2"}))
	require.NoError(t, scheduler.Remove(context.Background(), "scout-3"))
	assert.Equal(t, "workspace-a.scout-2", base.written[0].ScoutID)
	assert.Equal(t, []string{"workspace-a.scout-3"}, base.removed)
}

func TestLegacyScoped_DoesNotSeeWorkspaceAlarms(t *testing.T) {
	base := &recordingAlarms{entries: []domain.ScoutAlarmEntry{
		{ScoutID: "legacy-scout", Fingerprint: "legacy"},
		{ScoutID: "workspace-a.scout-1", Fingerprint: "a"},
	}}
	scheduler := LegacyScoped(base)

	entries, err := scheduler.List(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "legacy-scout", entries[0].ScoutID)

	require.NoError(t, scheduler.Write(context.Background(), domain.ScoutAlarm{ScoutID: "legacy-new"}))
	require.NoError(t, scheduler.Remove(context.Background(), "legacy-old"))
	assert.Equal(t, "legacy-new", base.written[0].ScoutID)
	assert.Equal(t, []string{"legacy-old"}, base.removed)
}
