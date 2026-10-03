package services

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fakeNotificationRepository keeps notifications in memory.
type fakeNotificationRepository struct {
	mu            sync.Mutex
	notifications []*domain.Notification
	saveErr       error
}

func (f *fakeNotificationRepository) Save(_ context.Context, notification *domain.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	notification.ID = string(rune('a' + len(f.notifications)))
	stored := *notification
	f.notifications = append(f.notifications, &stored)
	return nil
}

func (f *fakeNotificationRepository) List(_ context.Context, unseen bool, _ int) ([]*domain.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Notification
	for i := len(f.notifications) - 1; i >= 0; i-- {
		if n := f.notifications[i]; !unseen || n.SeenAt == nil {
			read := *n
			out = append(out, &read)
		}
	}
	return out, nil
}

func (f *fakeNotificationRepository) ListUnposted(_ context.Context, since time.Time) ([]*domain.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Notification
	for _, n := range f.notifications {
		if n.PostedAt == nil && !n.CreatedAt.Before(since) {
			read := *n
			out = append(out, &read)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.Before(out[b].CreatedAt) })
	return out, nil
}

func (f *fakeNotificationRepository) ClaimPosted(_ context.Context, id string, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.notifications {
		if n.ID == id && n.PostedAt == nil {
			n.PostedAt = &at
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeNotificationRepository) MarkSeen(_ context.Context, id string, at time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	marked := 0
	for _, n := range f.notifications {
		if n.SeenAt == nil && (id == "" || n.ID == id) {
			n.SeenAt = &at
			marked++
		}
	}
	if id != "" && marked == 0 {
		return 0, &ports.NotFoundError{Kind: "notification", ID: id}
	}
	return marked, nil
}

func (f *fakeNotificationRepository) all() []*domain.Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*domain.Notification(nil), f.notifications...)
}

// fakeDesktopApp records the deliveries it was asked to start.
type fakeDesktopApp struct {
	mu          sync.Mutex
	executables []string
	err         error
}

func (f *fakeDesktopApp) DeliverNotifications(_ context.Context, executable string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executables = append(f.executables, executable)
	return f.err
}

func (f *fakeDesktopApp) started() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.executables...)
}

type notificationFixture struct {
	svc      *NotificationService
	repo     *fakeNotificationRepository
	desktop  *fakeDesktopApp
	settings *SettingsService
	clock    *clock
}

func newNotificationFixture(t *testing.T) *notificationFixture {
	t.Helper()
	repo := &fakeNotificationRepository{}
	desktop := &fakeDesktopApp{}
	settings := NewSettingsService(newFakeSettingsRepository())
	c := &clock{now: time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)}
	svc := NewNotificationService(repo, settings, desktop)
	svc.now = c.Now
	return &notificationFixture{svc: svc, repo: repo, desktop: desktop, settings: settings, clock: c}
}

func scheduledFind(scores ...int) domain.ScoutRunOutcome {
	report := &domain.ScoutRunReport{RunID: "run-1"}
	for _, score := range scores {
		report.Verdicts = append(report.Verdicts, domain.ScoutVerdict{Eligible: true, HasScore: true, Score: score})
	}
	return domain.ScoutRunOutcome{ScoutID: "scout-1", ScoutName: "Mornings", RunID: "run-1",
		Trigger: domain.RunTriggerSchedule, Report: report}
}

func TestNotifications_ARunWorthTellingIsRecordedAndPosted(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	ctx := context.Background()
	require.NoError(t, f.svc.RegisterDesktop(ctx, "/Applications/JobPilot.app/Contents/MacOS/jobpilot-desktop"))

	f.svc.ScoutRunFinished(ctx, scheduledFind(72, 91))
	f.svc.Wait()

	stored := f.repo.all()
	require.Len(t, stored, 1)
	assert.Equal(t, "Mornings found 2 jobs for you", stored[0].Title)
	assert.Equal(t, f.clock.Now(), stored[0].CreatedAt)
	assert.Equal(t, []string{"/Applications/JobPilot.app/Contents/MacOS/jobpilot-desktop"}, f.desktop.started())
}

func TestNotifications_ARunNotWorthTellingSaysNothing(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	ctx := context.Background()
	require.NoError(t, f.svc.RegisterDesktop(ctx, "/opt/jobpilot-desktop"))

	run := scheduledFind(72)
	run.Trigger = domain.RunTriggerManual
	f.svc.ScoutRunFinished(ctx, run)
	f.svc.Wait()

	assert.Empty(t, f.repo.all())
	assert.Empty(t, f.desktop.started())
}

func TestNotifications_TheStrongMatchScoreIsTheUsers(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	ctx := context.Background()
	require.NoError(t, f.svc.SaveSettings(ctx, &domain.NotificationSettings{Enabled: true, StrongMatchScore: 70}))

	run := scheduledFind(72)
	run.Trigger = domain.RunTriggerManual
	f.svc.ScoutRunFinished(ctx, run)

	require.Len(t, f.repo.all(), 1, "72 is strong when the user says 70 is")
}

func TestNotifications_TurnedOffStillRecordsButPostsNothing(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	ctx := context.Background()
	require.NoError(t, f.svc.RegisterDesktop(ctx, "/opt/jobpilot-desktop"))
	require.NoError(t, f.svc.SaveSettings(ctx, &domain.NotificationSettings{Enabled: false, StrongMatchScore: 80}))

	f.svc.ScoutRunFinished(ctx, scheduledFind(90))
	f.svc.Wait()

	assert.Len(t, f.repo.all(), 1, "the app still shows what the user has not seen")
	assert.Empty(t, f.desktop.started())
	postable, err := f.svc.Postable(ctx)
	require.NoError(t, err)
	assert.Empty(t, postable)
}

func TestNotifications_WithoutTheAppsLocationNothingIsStarted(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)

	f.svc.ScoutRunFinished(context.Background(), scheduledFind(90))
	f.svc.Wait()

	assert.Len(t, f.repo.all(), 1)
	assert.Empty(t, f.desktop.started(), "a daemon the app never told where it is posts nothing")
}

func TestNotifications_AFailureToRecordDoesNotPanicOrPost(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	f.repo.saveErr = errors.New("disk full")
	require.NoError(t, f.svc.RegisterDesktop(context.Background(), "/opt/jobpilot-desktop"))

	f.svc.ScoutRunFinished(context.Background(), scheduledFind(90))
	f.svc.Wait()

	assert.Empty(t, f.desktop.started())
}

func TestNotifications_Postable(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	ctx := context.Background()
	f.svc.ScoutRunFinished(ctx, scheduledFind(90))
	f.clock.Set(f.clock.Now().Add(2 * time.Hour))
	f.svc.ScoutRunFinished(ctx, scheduledFind(91))

	postable, err := f.svc.Postable(ctx)

	require.NoError(t, err)
	require.Len(t, postable, 1, "the one from two hours ago waits in the app instead")
	assert.Equal(t, f.clock.Now(), postable[0].CreatedAt)
}

func TestNotifications_OnlyTheFirstClaimPosts(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	ctx := context.Background()
	f.svc.ScoutRunFinished(ctx, scheduledFind(90))
	id := f.repo.all()[0].ID

	first, err := f.svc.ClaimPosted(ctx, id)
	require.NoError(t, err)
	second, err := f.svc.ClaimPosted(ctx, id)
	require.NoError(t, err)

	assert.True(t, first)
	assert.False(t, second, "two deliveries never post one notification twice")
}

func TestNotifications_MarkSeen(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	ctx := context.Background()
	f.svc.ScoutRunFinished(ctx, scheduledFind(90))
	f.svc.ScoutRunFinished(ctx, scheduledFind(91))

	unseen, err := f.svc.List(ctx, true, 0)
	require.NoError(t, err)
	require.Len(t, unseen, 2)

	marked, err := f.svc.MarkSeen(ctx, unseen[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 1, marked)
	marked, err = f.svc.MarkSeen(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 1, marked)

	unseen, err = f.svc.List(ctx, true, 0)
	require.NoError(t, err)
	assert.Empty(t, unseen)
}

func TestNotifications_RegisterDesktopNeedsAnAbsolutePath(t *testing.T) {
	t.Parallel()
	f := newNotificationFixture(t)
	assert.Error(t, f.svc.RegisterDesktop(context.Background(), "jobpilot-desktop"))
}

// recordingNotifier records how the scout service said each run ended.
type recordingNotifier struct {
	mu   sync.Mutex
	runs []domain.ScoutRunOutcome
}

func (r *recordingNotifier) ScoutRunFinished(_ context.Context, run domain.ScoutRunOutcome) {
	r.mu.Lock()
	r.runs = append(r.runs, run)
	r.mu.Unlock()
}

func (r *recordingNotifier) told() []domain.ScoutRunOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.ScoutRunOutcome(nil), r.runs...)
}

func TestScoutRuns_TellTheNotifierHowTheyEnded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("a manual run, with what it created", func(t *testing.T) {
		t.Parallel()
		f, _, _ := scheduledFixture(t, nil)
		notifier := &recordingNotifier{}
		f.svc.WithNotifier(notifier)

		report, err := f.svc.Run(ctx, f.scout.ID)
		require.NoError(t, err)

		told := notifier.told()
		require.Len(t, told, 1)
		assert.Equal(t, domain.RunTriggerManual, told[0].Trigger)
		assert.Equal(t, "Daily Go", told[0].ScoutName)
		assert.Equal(t, report.RunID, told[0].RunID)
		assert.False(t, told[0].Failed)
		assert.Equal(t, 1, told[0].Report.Created)
	})

	t.Run("a scheduled run that could not start is a failure worth telling", func(t *testing.T) {
		t.Parallel()
		f, _, c := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
		notifier := &recordingNotifier{}
		f.svc.WithNotifier(notifier)
		f.finder.listErr = errors.New("database is locked")
		c.Set(c.Now().Add(24 * time.Hour))

		wake, err := f.svc.Wake(ctx, f.scout.ID)
		require.NoError(t, err)
		require.Equal(t, domain.WakeStarted, wake.Outcome)
		f.svc.Wait()

		told := notifier.told()
		require.Len(t, told, 1)
		assert.Equal(t, domain.RunTriggerSchedule, told[0].Trigger)
		assert.True(t, told[0].Failed)
	})

	t.Run("a scheduled run that turned out not to be due tells nothing", func(t *testing.T) {
		t.Parallel()
		f, _, c := scheduledFixture(t, func(s *domain.Scout) { s.Schedule = daily(8) })
		notifier := &recordingNotifier{}
		f.svc.WithNotifier(notifier)
		c.Set(c.Now().Add(24 * time.Hour))

		f.svc.runLock <- struct{}{}
		wake, err := f.svc.Wake(ctx, f.scout.ID)
		require.NoError(t, err)
		require.Equal(t, domain.WakeStarted, wake.Outcome)
		require.NoError(t, f.runs.Save(ctx, &domain.AgentRun{ScoutID: f.scout.ID, TaskType: ScoutTaskType,
			Status: domain.AgentRunStatusCompleted, StartedAt: c.Now()}))
		<-f.svc.runLock
		f.svc.Wait()

		assert.Empty(t, notifier.told())
	})
}
