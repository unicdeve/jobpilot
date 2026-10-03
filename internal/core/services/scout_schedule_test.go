package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fakeScoutAlarms is an in-memory operating-system scheduler.
type fakeScoutAlarms struct {
	mu        sync.Mutex
	entries   map[string]domain.ScoutAlarmEntry
	alarms    map[string]domain.ScoutAlarm
	writes    int
	removed   []string
	listErr   error
	writeErrs map[string]error
}

func newFakeScoutAlarms() *fakeScoutAlarms {
	return &fakeScoutAlarms{
		entries:   map[string]domain.ScoutAlarmEntry{},
		alarms:    map[string]domain.ScoutAlarm{},
		writeErrs: map[string]error{},
	}
}

func (f *fakeScoutAlarms) List(context.Context) ([]domain.ScoutAlarmEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]domain.ScoutAlarmEntry, 0, len(f.entries))
	for _, entry := range f.entries {
		out = append(out, entry)
	}
	return out, nil
}

func (f *fakeScoutAlarms) Write(_ context.Context, alarm domain.ScoutAlarm) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.writeErrs[alarm.ScoutID]; err != nil {
		return err
	}
	f.writes++
	f.alarms[alarm.ScoutID] = alarm
	// Write keeps an alarm the user switched off switched off.
	f.entries[alarm.ScoutID] = domain.ScoutAlarmEntry{
		ScoutID: alarm.ScoutID, Fingerprint: alarm.Fingerprint(),
		SwitchedOff: f.entries[alarm.ScoutID].SwitchedOff,
	}
	return nil
}

func (f *fakeScoutAlarms) Remove(_ context.Context, scoutID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, scoutID)
	delete(f.entries, scoutID)
	delete(f.alarms, scoutID)
	return nil
}

func (f *fakeScoutAlarms) has(scoutID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.entries[scoutID]
	return ok
}

// clock is a settable now, for a service whose decisions depend on it.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

var alarmTarget = AlarmTarget{Executable: "/opt/jobpilot/jobpilot", Env: map[string]string{"HOME": "/home/ada"}}

// scheduledFixture is a scout fixture whose service has a clock and an
// operating-system scheduler.
func scheduledFixture(t *testing.T, adjust func(*domain.Scout)) (*scoutFixture, *fakeScoutAlarms, *clock) {
	t.Helper()
	f := newScoutFixture(t, []*domain.Job{job("job-1", "Go Engineer", "Acme")},
		map[string]int{"job-1": 90}, domain.SearchCriteria{Keywords: "Go"}, adjust)
	alarms := newFakeScoutAlarms()
	c := &clock{now: time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)}
	f.svc.now = c.Now
	f.svc.WithAlarms(alarms, alarmTarget)
	return f, alarms, c
}

func daily(hour int) domain.ScoutSchedule {
	return domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: hour * 60}
}

func newScout(searchID string, schedule domain.ScoutSchedule) *domain.Scout {
	return &domain.Scout{
		Name: "Mornings", SavedSearchID: searchID, MinMatchScore: 70,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true, Schedule: schedule,
	}
}

func TestScoutSchedule_CreatingWritesTheAlarm(t *testing.T) {
	t.Parallel()
	f, alarms, c := scheduledFixture(t, nil)
	ctx := context.Background()

	scout := newScout(f.searchID, daily(8))
	require.NoError(t, f.svc.CreateScout(ctx, scout))

	require.NotNil(t, scout.ScheduleSetAt)
	assert.Equal(t, c.Now(), *scout.ScheduleSetAt, "the first run is measured from when the schedule was set")
	require.True(t, alarms.has(scout.ID))
	written := alarms.alarms[scout.ID]
	assert.Equal(t, alarmTarget.Executable, written.Executable)
	assert.Equal(t, daily(8), written.Schedule)
	assert.Equal(t, alarmTarget.Env, written.Env)

	require.NotNil(t, scout.Alarm)
	assert.Equal(t, domain.AlarmScheduled, scout.Alarm.State)
	require.NotNil(t, scout.NextRunAt)
	assert.Equal(t, time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC), *scout.NextRunAt,
		"scheduled at 10:00 for 08:00, so tomorrow")
}

func TestScoutSchedule_AnUnscheduledScoutHasNoAlarm(t *testing.T) {
	t.Parallel()
	f, alarms, _ := scheduledFixture(t, nil)

	scout := newScout(f.searchID, domain.ScoutSchedule{})
	require.NoError(t, f.svc.CreateScout(context.Background(), scout))

	assert.Equal(t, domain.ScheduleOff, scout.Schedule.Kind)
	assert.Nil(t, scout.ScheduleSetAt)
	assert.False(t, alarms.has(scout.ID))
	assert.Nil(t, scout.Alarm)
	assert.Nil(t, scout.NextRunAt)
}

func TestScoutSchedule_UpdatingKeepsWhatTheUserCannotChange(t *testing.T) {
	t.Parallel()
	f, alarms, c := scheduledFixture(t, nil)
	ctx := context.Background()
	scout := newScout(f.searchID, daily(8))
	require.NoError(t, f.svc.CreateScout(ctx, scout))
	set := *scout.ScheduleSetAt
	ran := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, f.scouts.MarkRun(ctx, scout.ID, ran))

	t.Run("an edit is not a run, and does not erase the last one", func(t *testing.T) {
		c.Set(set.Add(time.Hour))
		edit := newScout(f.searchID, daily(8))
		edit.ID, edit.Name = scout.ID, "Weekday mornings"
		require.NoError(t, f.svc.UpdateScout(ctx, edit))

		stored := f.scouts.scouts[scout.ID]
		require.NotNil(t, stored.LastRunAt)
		assert.Equal(t, ran, *stored.LastRunAt)
		assert.Equal(t, set, *stored.ScheduleSetAt, "renaming a scout does not move its next run")
	})

	t.Run("changing the schedule sets it again and rewrites the alarm", func(t *testing.T) {
		c.Set(set.Add(2 * time.Hour))
		edit := newScout(f.searchID, daily(9))
		edit.ID = scout.ID
		require.NoError(t, f.svc.UpdateScout(ctx, edit))

		assert.Equal(t, c.Now(), *f.scouts.scouts[scout.ID].ScheduleSetAt)
		assert.Equal(t, daily(9), alarms.alarms[scout.ID].Schedule)
	})

	t.Run("pausing removes the alarm and keeps the schedule", func(t *testing.T) {
		edit := newScout(f.searchID, daily(9))
		edit.ID, edit.IsEnabled = scout.ID, false
		require.NoError(t, f.svc.UpdateScout(ctx, edit))

		assert.False(t, alarms.has(scout.ID))
		assert.Equal(t, daily(9), f.scouts.scouts[scout.ID].Schedule)
	})

	t.Run("turning the schedule off removes the alarm", func(t *testing.T) {
		edit := newScout(f.searchID, daily(9))
		edit.ID = scout.ID
		require.NoError(t, f.svc.UpdateScout(ctx, edit))
		require.True(t, alarms.has(scout.ID))

		edit.Schedule = domain.ScoutSchedule{Kind: domain.ScheduleOff}
		require.NoError(t, f.svc.UpdateScout(ctx, edit))
		assert.False(t, alarms.has(scout.ID))
		assert.Nil(t, f.scouts.scouts[scout.ID].ScheduleSetAt)
	})

	t.Run("a scout that does not exist is not found", func(t *testing.T) {
		edit := newScout(f.searchID, daily(9))
		edit.ID = "missing"
		assert.ErrorIs(t, f.svc.UpdateScout(ctx, edit), ports.ErrNotFound)
	})
}

func TestScoutSchedule_DeletingRemovesTheAlarm(t *testing.T) {
	t.Parallel()
	f, alarms, _ := scheduledFixture(t, nil)
	ctx := context.Background()
	scout := newScout(f.searchID, daily(8))
	require.NoError(t, f.svc.CreateScout(ctx, scout))

	require.NoError(t, f.svc.DeleteScout(ctx, scout.ID))

	assert.False(t, alarms.has(scout.ID))
}

func TestScoutSchedule_ASaveSurvivesAnAlarmThatCannotBeWritten(t *testing.T) {
	t.Parallel()
	f, alarms, _ := scheduledFixture(t, nil)
	ctx := context.Background()
	scout := newScout(f.searchID, daily(8))
	scout.ID = "scout-unwritable"
	alarms.writeErrs[scout.ID] = errors.New("launchctl: Bootstrap failed: 5: Input/output error")

	require.NoError(t, f.svc.CreateScout(ctx, scout), "the scout is saved; its alarm is the operating system's problem")

	_, stored := f.scouts.scouts[scout.ID]
	assert.True(t, stored)
	require.NotNil(t, scout.Alarm)
	assert.Equal(t, domain.AlarmMissing, scout.Alarm.State)
	assert.Contains(t, scout.Alarm.Reason, "Bootstrap failed")
}

func TestScoutSchedule_Reconcile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("puts back a removed alarm, rewrites a moved one and removes orphans", func(t *testing.T) {
		t.Parallel()
		f, alarms, _ := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
		other := newScout(f.searchID, domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 6})
		other.Name = "Often"
		require.NoError(t, f.scouts.Save(ctx, other))

		stale := domain.ScoutAlarm{ScoutID: other.ID, Schedule: other.Schedule, Executable: "/old/jobpilot"}
		alarms.entries[other.ID] = domain.ScoutAlarmEntry{ScoutID: other.ID, Fingerprint: stale.Fingerprint()}
		alarms.entries["gone"] = domain.ScoutAlarmEntry{ScoutID: "gone", Fingerprint: "x"}

		require.NoError(t, f.svc.Reconcile(ctx))

		assert.True(t, alarms.has(f.scout.ID), "a scheduled scout with no alarm gets one")
		assert.Equal(t, alarmTarget.Executable, alarms.alarms[other.ID].Executable, "a moved executable is rewritten")
		assert.False(t, alarms.has("gone"), "an alarm for no scout is removed")
		assert.Equal(t, []string{"gone"}, alarms.removed)
	})

	t.Run("leaves a matching alarm alone", func(t *testing.T) {
		t.Parallel()
		f, alarms, _ := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
		require.NoError(t, f.svc.Reconcile(ctx))
		require.Equal(t, 1, alarms.writes)

		require.NoError(t, f.svc.Reconcile(ctx))
		assert.Equal(t, 1, alarms.writes)
	})

	t.Run("reports an alarm switched off outside JobPilot and leaves it off", func(t *testing.T) {
		t.Parallel()
		f, alarms, _ := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
		require.NoError(t, f.svc.Reconcile(ctx))
		entry := alarms.entries[f.scout.ID]
		entry.SwitchedOff = true
		alarms.entries[f.scout.ID] = entry

		require.NoError(t, f.svc.Reconcile(ctx))

		scout, err := f.svc.GetScout(ctx, f.scout.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.AlarmSwitchedOff, scout.Alarm.State)
		assert.True(t, alarms.entries[f.scout.ID].SwitchedOff)
	})

	t.Run("one alarm that fails does not stop the others", func(t *testing.T) {
		t.Parallel()
		f, alarms, _ := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
		other := newScout(f.searchID, daily(9))
		other.Name = "Later"
		require.NoError(t, f.scouts.Save(ctx, other))
		alarms.writeErrs[f.scout.ID] = errors.New("denied")

		err := f.svc.Reconcile(ctx)

		require.ErrorContains(t, err, "denied")
		assert.True(t, alarms.has(other.ID))
	})

	t.Run("a system with no scheduler says so on each scheduled scout", func(t *testing.T) {
		t.Parallel()
		f, alarms, _ := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
		alarms.listErr = ports.ErrAlarmsUnavailable

		require.NoError(t, f.svc.Reconcile(ctx), "no scheduler is a fact about the system, not a failure")

		scout, err := f.svc.GetScout(ctx, f.scout.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.AlarmUnavailable, scout.Alarm.State)
	})

	t.Run("without a scheduler at all, a schedule is kept and said to be unavailable", func(t *testing.T) {
		t.Parallel()
		f := newScoutFixture(t, nil, nil, domain.SearchCriteria{}, func(s *domain.Scout) { s.Schedule = daily(8) })

		require.NoError(t, f.svc.Reconcile(ctx))
		scout, err := f.svc.GetScout(ctx, f.scout.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.AlarmUnavailable, scout.Alarm.State)
	})
}

func TestScoutSchedule_RemoveAllAlarms(t *testing.T) {
	t.Parallel()
	f, alarms, _ := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
	ctx := context.Background()
	require.NoError(t, f.svc.Reconcile(ctx))
	alarms.entries["orphan"] = domain.ScoutAlarmEntry{ScoutID: "orphan"}

	require.NoError(t, f.svc.RemoveAllAlarms(ctx))

	assert.Empty(t, alarms.entries)
}

func TestScoutWake(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("a paused scout does not run", func(t *testing.T) {
		t.Parallel()
		f, _, _ := scheduledFixture(t, func(s *domain.Scout) { s.Schedule, s.IsEnabled = daily(8), false })
		wake, err := f.svc.Wake(ctx, f.scout.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.WakeDisabled, wake.Outcome)
	})

	t.Run("an unscheduled scout does not run", func(t *testing.T) {
		t.Parallel()
		f, _, _ := scheduledFixture(t, nil)
		wake, err := f.svc.Wake(ctx, f.scout.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.WakeUnscheduled, wake.Outcome)
	})

	t.Run("a scout whose time has not come says when it will", func(t *testing.T) {
		t.Parallel()
		f, _, c := scheduledFixture(t, nil)
		scout := newScout(f.searchID, daily(8))
		require.NoError(t, f.svc.CreateScout(ctx, scout))

		wake, err := f.svc.Wake(ctx, scout.ID)

		require.NoError(t, err)
		assert.Equal(t, domain.WakeNotDue, wake.Outcome)
		require.NotNil(t, wake.NextRunAt)
		assert.Equal(t, time.Date(2026, 10, 3, 8, 0, 0, 0, c.Now().Location()), *wake.NextRunAt)
		assert.Empty(t, f.runs.runs, "nothing ran")
	})

	t.Run("a due scout runs, on its own, and says so", func(t *testing.T) {
		t.Parallel()
		f, _, c := scheduledFixture(t, nil)
		scout := newScout(f.searchID, daily(8))
		require.NoError(t, f.svc.CreateScout(ctx, scout))
		c.Set(time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC))

		wake, err := f.svc.Wake(ctx, scout.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.WakeStarted, wake.Outcome)
		f.svc.Wait()

		require.Len(t, f.runs.runs, 1)
		for _, run := range f.runs.runs {
			assert.Equal(t, domain.RunTriggerSchedule, run.Trigger)
			assert.Equal(t, domain.AgentRunStatusCompleted, run.Status)
			assert.Equal(t, scout.ID, run.ScoutID)
		}

		again, err := f.svc.Wake(ctx, scout.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.WakeNotDue, again.Outcome, "the run it just made counts")
	})

	t.Run("an alarm for a scout that is gone is not found, and its alarm is removed", func(t *testing.T) {
		t.Parallel()
		f, alarms, _ := scheduledFixture(t, nil)
		alarms.entries["gone"] = domain.ScoutAlarmEntry{ScoutID: "gone"}

		_, err := f.svc.Wake(ctx, "gone")

		require.ErrorIs(t, err, ports.ErrNotFound)
		assert.Eventually(t, func() bool { return !alarms.has("gone") }, time.Second, 5*time.Millisecond)
	})
}

// blockingMatcher holds every scoring call until released, so a test can
// look at a run while it is in progress.
type blockingMatcher struct {
	started chan string
	release chan struct{}
}

func newBlockingMatcher() *blockingMatcher {
	return &blockingMatcher{started: make(chan string, 8), release: make(chan struct{})}
}

func (m *blockingMatcher) AnalyzeJobForCandidate(ctx context.Context, candidateID, jobID string) (*domain.JobMatch, error) {
	m.started <- jobID
	select {
	case <-m.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &domain.JobMatch{JobID: jobID, CandidateID: candidateID, Score: 90}, nil
}

func TestScoutRuns_HappenOneAtATime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, _, c := scheduledFixture(t, nil)
	matcher := newBlockingMatcher()
	f.svc.matcher = matcher
	other := newScout(f.searchID, daily(8))
	other.Name = "Other"
	require.NoError(t, f.svc.CreateScout(ctx, other))

	first := make(chan error, 1)
	go func() {
		_, err := f.svc.Run(ctx, f.scout.ID)
		first <- err
	}()
	<-matcher.started

	scouts, err := f.svc.ListScouts(ctx)
	require.NoError(t, err)
	activity := map[string]domain.ScoutActivity{}
	for _, scout := range scouts {
		activity[scout.ID] = scout.Activity
	}
	assert.Equal(t, domain.ScoutRunning, activity[f.scout.ID])

	// The other scout falls due while the first runs, and its alarm fires.
	c.Set(time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC))
	wake, err := f.svc.Wake(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStarted, wake.Outcome)

	assert.Eventually(t, func() bool { return f.svc.activity(other.ID) == domain.ScoutWaiting },
		time.Second, 5*time.Millisecond, "it waits for the first rather than scoring beside it")
	select {
	case jobID := <-matcher.started:
		t.Fatalf("a second run started scoring %s while the first held the lock", jobID)
	case <-time.After(50 * time.Millisecond):
	}

	again, err := f.svc.Wake(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.WakeAlreadyRunning, again.Outcome, "a second alarm does not queue a second run")

	close(matcher.release)
	require.NoError(t, <-first)
	f.svc.Wait()

	assert.Len(t, f.runs.runs, 2)
	created, err := f.apps.ListByCandidate(ctx, "cand-1")
	require.NoError(t, err)
	assert.Len(t, created, 1, "the second run saw the application the first created")
}

func TestScoutRuns_AScheduledRunThatWaitedForAManualOneIsDropped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, _, c := scheduledFixture(t, nil)
	scout := newScout(f.searchID, domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: 6})
	require.NoError(t, f.svc.CreateScout(ctx, scout))
	c.Set(c.Now().Add(6 * time.Hour))

	// Another scout's run holds the lock when this one's alarm fires.
	f.svc.runLock <- struct{}{}
	wake, err := f.svc.Wake(ctx, scout.ID)
	require.NoError(t, err)
	require.Equal(t, domain.WakeStarted, wake.Outcome)

	// While it waits, the user runs it by hand, and that run goes first.
	manual := &domain.AgentRun{ScoutID: scout.ID, Trigger: domain.RunTriggerManual,
		TaskType: ScoutTaskType, Status: domain.AgentRunStatusCompleted, StartedAt: c.Now()}
	require.NoError(t, f.runs.Save(ctx, manual))

	<-f.svc.runLock
	f.svc.Wait()

	require.Len(t, f.runs.runs, 1, "the scheduled run found it no longer due, and recorded nothing")
	assert.Empty(t, f.svc.activity(scout.ID))
}

func TestScoutRun_GivesUpWaitingWhenItsRequestEnds(t *testing.T) {
	t.Parallel()
	f, _, _ := scheduledFixture(t, nil)
	matcher := newBlockingMatcher()
	f.svc.matcher = matcher
	ctx := context.Background()

	first := make(chan error, 1)
	go func() {
		_, err := f.svc.Run(ctx, f.scout.ID)
		first <- err
	}()
	<-matcher.started

	waiting, cancel := context.WithCancel(ctx)
	second := make(chan error, 1)
	go func() {
		_, err := f.svc.Run(waiting, f.scout.ID)
		second <- err
	}()
	assert.Eventually(t, func() bool {
		f.svc.mu.Lock()
		defer f.svc.mu.Unlock()
		return f.svc.waiting[f.scout.ID] == 1
	}, time.Second, 5*time.Millisecond)
	cancel()

	require.ErrorIs(t, <-second, context.Canceled)
	assert.Equal(t, domain.ScoutRunning, f.svc.activity(f.scout.ID), "the run in progress carries on")
	close(matcher.release)
	require.NoError(t, <-first)
	assert.Empty(t, f.svc.activity(f.scout.ID))
}
