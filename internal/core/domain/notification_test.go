package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// created is a report verdict for an application a run created.
func created(score int) ScoutVerdict {
	return ScoutVerdict{Eligible: true, HasScore: true, Score: score, Considered: true}
}

func rejected(score int) ScoutVerdict {
	return ScoutVerdict{HasScore: true, Score: score, Considered: true, Reason: "below the floor"}
}

func outcome(trigger RunTrigger, failed bool, verdicts ...ScoutVerdict) ScoutRunOutcome {
	return ScoutRunOutcome{
		ScoutID: "scout-1", ScoutName: "Morning Go roles", RunID: "run-1", Trigger: trigger,
		Report: &ScoutRunReport{Verdicts: verdicts}, Failed: failed,
	}
}

func TestScoutRunNotification(t *testing.T) {
	cases := []struct {
		name      string
		run       ScoutRunOutcome
		wantKind  NotificationKind
		wantTitle string
		wantBody  string
	}{
		{
			name:      "a scheduled run that created applications",
			run:       outcome(RunTriggerSchedule, false, created(72), created(75), rejected(90)),
			wantKind:  NotificationScoutFound,
			wantTitle: "Morning Go roles found 2 jobs for you",
			wantBody:  "They’re ready for review.",
		},
		{
			name:      "a scheduled run with some strong matches says both, once",
			run:       outcome(RunTriggerSchedule, false, created(72), created(88), created(91)),
			wantKind:  NotificationScoutFound,
			wantTitle: "Morning Go roles found 3 jobs for you",
			wantBody:  "2 of them are strong matches. They’re ready for review.",
		},
		{
			name:      "one strong match among several",
			run:       outcome(RunTriggerSchedule, false, created(72), created(88)),
			wantKind:  NotificationScoutFound,
			wantTitle: "Morning Go roles found 2 jobs for you",
			wantBody:  "1 of them is a strong match. They’re ready for review.",
		},
		{
			name:      "a manual run tells only about a strong match",
			run:       outcome(RunTriggerManual, false, created(85)),
			wantKind:  NotificationScoutFound,
			wantTitle: "Morning Go roles found 1 job for you",
			wantBody:  "It’s a strong match, ready for review.",
		},
		{
			name:      "every one strong",
			run:       outcome(RunTriggerManual, false, created(85), created(80)),
			wantKind:  NotificationScoutFound,
			wantTitle: "Morning Go roles found 2 jobs for you",
			wantBody:  "All of them are strong matches, ready for review.",
		},
		{
			name:      "a scheduled run that failed",
			run:       outcome(RunTriggerSchedule, true),
			wantKind:  NotificationScoutFailed,
			wantTitle: "Morning Go roles couldn’t run",
			wantBody:  "Open JobPilot to see why.",
		},
		{
			name:      "a scheduled run that failed after creating something tells what it created",
			run:       outcome(RunTriggerSchedule, true, created(70)),
			wantKind:  NotificationScoutFound,
			wantTitle: "Morning Go roles found 1 job for you",
			wantBody:  "It’s ready for review.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoutRunNotification(tc.run, 80)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantKind, got.Kind)
			assert.Equal(t, tc.wantTitle, got.Title)
			assert.Equal(t, tc.wantBody, got.Body)
			assert.Equal(t, "scout-1", got.ScoutID)
			assert.Equal(t, "run-1", got.RunID)
		})
	}
}

func TestScoutRunNotification_SaysNothing(t *testing.T) {
	cases := map[string]ScoutRunOutcome{
		"a scheduled run that found nothing":            outcome(RunTriggerSchedule, false, rejected(95)),
		"a manual run with no strong match":             outcome(RunTriggerManual, false, created(79)),
		"a manual run that failed":                      outcome(RunTriggerManual, true),
		"a strong score on a job it did not create for": outcome(RunTriggerManual, false, rejected(99)),
		"a run with no report":                          {Trigger: RunTriggerManual},
	}
	for name, run := range cases {
		assert.Nil(t, ScoutRunNotification(run, 80), name)
	}
}

func TestScoutRunNotification_TheThresholdIsTheUsers(t *testing.T) {
	run := outcome(RunTriggerManual, false, created(70))
	assert.Nil(t, ScoutRunNotification(run, 80))
	assert.NotNil(t, ScoutRunNotification(run, 70), "a score at the threshold is strong")
}

// TestScoutRunNotification_SaysNothingAboutTheJob is ADR-0028 §3: a
// notification shows on a lock screen.
func TestScoutRunNotification_SaysNothingAboutTheJob(t *testing.T) {
	run := outcome(RunTriggerSchedule, false, ScoutVerdict{
		JobID: "job-1", JobTitle: "Senior Go Engineer at Acme", Eligible: true, HasScore: true, Score: 93,
		Reason: "match score 93 clears the scout's floor of 70",
	})
	got := ScoutRunNotification(run, 80)
	require.NotNil(t, got)
	for _, leak := range []string{"Senior", "Acme", "93", "floor"} {
		assert.NotContains(t, got.Title+got.Body, leak)
	}
}

func TestNotificationPostable(t *testing.T) {
	now := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
	fresh := Notification{CreatedAt: now.Add(-10 * time.Minute)}
	stale := Notification{CreatedAt: now.Add(-2 * time.Hour)}
	posted := Notification{CreatedAt: now, PostedAt: &now}

	assert.True(t, fresh.Postable(now))
	assert.False(t, stale.Postable(now), "this morning's news waits in the app rather than arriving at dinner")
	assert.False(t, posted.Postable(now))
}

func TestNotificationSettings(t *testing.T) {
	assert.Equal(t, NotificationSettings{Enabled: true, StrongMatchScore: 80}, DefaultNotificationSettings())
	assert.NoError(t, DefaultNotificationSettings().Validate())
	assert.Error(t, NotificationSettings{StrongMatchScore: 0}.Validate())
	assert.Error(t, NotificationSettings{StrongMatchScore: 101}.Validate())
}
