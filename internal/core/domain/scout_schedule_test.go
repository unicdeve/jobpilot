package domain

import (
	"testing"
	"time"
	_ "time/tzdata" // the daylight-saving case needs a real zone on every platform

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func at(t *testing.T, loc *time.Location, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, loc)
	require.NoError(t, err)
	return parsed
}

func TestScoutScheduleValidate(t *testing.T) {
	cases := []struct {
		name     string
		schedule ScoutSchedule
		wantErr  string
	}{
		{"empty is off", ScoutSchedule{}, ""},
		{"off", ScoutSchedule{Kind: ScheduleOff}, ""},
		{"every hour", ScoutSchedule{Kind: ScheduleEvery, EveryHours: 1}, ""},
		{"every day's worth", ScoutSchedule{Kind: ScheduleEvery, EveryHours: 24}, ""},
		{"every zero hours", ScoutSchedule{Kind: ScheduleEvery}, "every 1 to 24 hours"},
		{"every 25 hours", ScoutSchedule{Kind: ScheduleEvery, EveryHours: 25}, "every 1 to 24 hours"},
		{"daily at midnight", ScoutSchedule{Kind: ScheduleDaily, DailyAt: 0}, ""},
		{"daily at 23:59", ScoutSchedule{Kind: ScheduleDaily, DailyAt: 23*60 + 59}, ""},
		{"daily at 24:00", ScoutSchedule{Kind: ScheduleDaily, DailyAt: 24 * 60}, "between 00:00 and 23:59"},
		{"daily before midnight", ScoutSchedule{Kind: ScheduleDaily, DailyAt: -1}, "between 00:00 and 23:59"},
		{"cron", ScoutSchedule{Kind: "0 8 * * *"}, "unknown schedule"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.schedule.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestScoutScheduleNormalized(t *testing.T) {
	assert.Equal(t, ScoutSchedule{Kind: ScheduleOff}, ScoutSchedule{}.Normalized())
	assert.Equal(t, ScoutSchedule{Kind: ScheduleOff}, ScoutSchedule{Kind: ScheduleOff, EveryHours: 6}.Normalized())
	assert.Equal(t, ScoutSchedule{Kind: ScheduleEvery, EveryHours: 6},
		ScoutSchedule{Kind: ScheduleEvery, EveryHours: 6, DailyAt: 480}.Normalized())
	assert.Equal(t, ScoutSchedule{Kind: ScheduleDaily, DailyAt: 480},
		ScoutSchedule{Kind: ScheduleDaily, EveryHours: 6, DailyAt: 480}.Normalized())
}

func TestScoutScheduleEvery(t *testing.T) {
	loc := time.UTC
	every6 := ScoutSchedule{Kind: ScheduleEvery, EveryHours: 6}
	since := at(t, loc, "2026-10-02 10:00:00")

	t.Run("due six hours after the last start, less the grace", func(t *testing.T) {
		assert.Equal(t, at(t, loc, "2026-10-02 15:55:00"), every6.DueAt(since))
		assert.False(t, every6.Due(at(t, loc, "2026-10-02 15:54:59"), since))
		assert.True(t, every6.Due(at(t, loc, "2026-10-02 15:55:00"), since))
		assert.True(t, every6.Due(at(t, loc, "2026-10-02 16:00:00"), since))
	})

	t.Run("an alarm a few seconds early still counts", func(t *testing.T) {
		assert.True(t, every6.Due(at(t, loc, "2026-10-02 15:59:58"), since))
	})

	t.Run("a run that started late does not push the next past its hour", func(t *testing.T) {
		started := at(t, loc, "2026-10-02 16:00:03")
		assert.True(t, every6.Due(at(t, loc, "2026-10-02 22:00:00"), started))
	})

	t.Run("the next run is the first hourly alarm that finds it due", func(t *testing.T) {
		assert.Equal(t, at(t, loc, "2026-10-02 16:00:00"), every6.NextRun(at(t, loc, "2026-10-02 10:30:00"), since))
		started := at(t, loc, "2026-10-02 10:20:00")
		assert.Equal(t, at(t, loc, "2026-10-02 17:00:00"), every6.NextRun(at(t, loc, "2026-10-02 10:30:00"), started))
	})

	t.Run("an overdue scout runs at the next alarm", func(t *testing.T) {
		now := at(t, loc, "2026-10-05 09:12:00")
		assert.True(t, every6.Due(now, since))
		assert.Equal(t, at(t, loc, "2026-10-05 10:00:00"), every6.NextRun(now, since))
	})
}

func TestScoutScheduleDaily(t *testing.T) {
	loc := time.UTC
	daily8 := ScoutSchedule{Kind: ScheduleDaily, DailyAt: 8 * 60}

	t.Run("scheduled after today's time, first runs tomorrow", func(t *testing.T) {
		set := at(t, loc, "2026-10-02 10:00:00")
		assert.False(t, daily8.Due(at(t, loc, "2026-10-02 10:00:01"), set))
		assert.False(t, daily8.Due(at(t, loc, "2026-10-03 07:54:59"), set))
		assert.True(t, daily8.Due(at(t, loc, "2026-10-03 08:00:00"), set))
		assert.Equal(t, at(t, loc, "2026-10-03 08:00:00"), daily8.NextRun(at(t, loc, "2026-10-02 10:00:01"), set))
	})

	t.Run("scheduled before today's time, runs today", func(t *testing.T) {
		set := at(t, loc, "2026-10-02 07:00:00")
		assert.True(t, daily8.Due(at(t, loc, "2026-10-02 08:00:00"), set))
	})

	t.Run("a run its early alarm started has had today's", func(t *testing.T) {
		started := at(t, loc, "2026-10-02 07:59:58")
		assert.False(t, daily8.Due(at(t, loc, "2026-10-02 08:01:00"), started))
		assert.True(t, daily8.Due(at(t, loc, "2026-10-03 08:00:00"), started))
	})

	t.Run("a manual run before the time does not stand in for it", func(t *testing.T) {
		started := at(t, loc, "2026-10-02 07:30:00")
		assert.True(t, daily8.Due(at(t, loc, "2026-10-02 08:00:00"), started))
	})

	t.Run("days asleep run once, then wait for tomorrow", func(t *testing.T) {
		last := at(t, loc, "2026-09-29 08:00:00")
		woke := at(t, loc, "2026-10-02 12:00:00")
		assert.True(t, daily8.Due(woke, last))
		assert.False(t, daily8.Due(woke.Add(time.Minute), woke))
		assert.True(t, daily8.Due(at(t, loc, "2026-10-03 08:00:00"), woke))
	})

	t.Run("an overdue scout shows its catch-up as the next run", func(t *testing.T) {
		last := at(t, loc, "2026-09-29 08:00:00")
		now := at(t, loc, "2026-10-02 12:00:00")
		assert.Equal(t, at(t, loc, "2026-10-03 08:00:00"), daily8.NextRun(now, last))
	})

	t.Run("the time is wall-clock across a change to daylight saving", func(t *testing.T) {
		ny, err := time.LoadLocation("America/New_York")
		require.NoError(t, err)
		// 2026-03-08 is 23 hours long in New York.
		last := at(t, ny, "2026-03-07 08:00:00")
		assert.Equal(t, at(t, ny, "2026-03-08 08:00:00"), daily8.NextRun(last.Add(time.Hour), last))
		assert.False(t, daily8.Due(at(t, ny, "2026-03-08 07:00:00"), last))
		assert.True(t, daily8.Due(at(t, ny, "2026-03-08 08:00:00"), last))
	})
}

// TestScoutScheduleDaily_InTheComputersTimeZone is the bug the desktop
// showed: read back from SQLite, a scout's times are in UTC, and a daily
// time worked out in UTC put "daily at 08:00" at 09:00 in Lagos.
func TestScoutScheduleDaily_InTheComputersTimeZone(t *testing.T) {
	lagos, err := time.LoadLocation("Africa/Lagos")
	require.NoError(t, err)
	daily8 := ScoutSchedule{Kind: ScheduleDaily, DailyAt: 8 * 60}
	set := time.Date(2026, 10, 2, 23, 20, 0, 0, time.UTC) // 00:20 in Lagos
	now := set.In(lagos)

	next := daily8.NextRun(now, set)
	assert.Equal(t, at(t, lagos, "2026-10-03 08:00:00"), next)
	assert.Equal(t, lagos, next.Location())
	assert.True(t, daily8.Due(at(t, lagos, "2026-10-03 08:00:00"), set))
	assert.False(t, daily8.Due(at(t, lagos, "2026-10-03 07:30:00"), set))
}

func TestScoutScheduleOff(t *testing.T) {
	now := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	assert.False(t, ScoutSchedule{Kind: ScheduleOff}.Due(now, time.Time{}))
	assert.False(t, ScoutSchedule{}.Due(now, time.Time{}))
}

func TestScoutScheduleNeverRun(t *testing.T) {
	now := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)
	assert.True(t, ScoutSchedule{Kind: ScheduleEvery, EveryHours: 6}.Due(now, time.Time{}))
}

func TestScoutAlarmFingerprint(t *testing.T) {
	base := ScoutAlarm{
		ScoutID:    "3f1c2a8e-0000-4000-8000-000000000001",
		Schedule:   ScoutSchedule{Kind: ScheduleDaily, DailyAt: 480},
		Executable: "/Applications/JobPilot.app/Contents/MacOS/jobpilot",
		Env:        map[string]string{"HOME": "/Users/me", "XDG_CONFIG_HOME": "/x"},
	}
	same := base
	same.Env = map[string]string{"XDG_CONFIG_HOME": "/x", "HOME": "/Users/me"}
	assert.Equal(t, base.Fingerprint(), same.Fingerprint(), "the order of the environment does not matter")

	for name, change := range map[string]func(*ScoutAlarm){
		"schedule":   func(a *ScoutAlarm) { a.Schedule.DailyAt = 540 },
		"kind":       func(a *ScoutAlarm) { a.Schedule = ScoutSchedule{Kind: ScheduleEvery, EveryHours: 8} },
		"executable": func(a *ScoutAlarm) { a.Executable = "/usr/local/bin/jobpilot" },
		"env":        func(a *ScoutAlarm) { a.Env = map[string]string{"HOME": "/Users/you"} },
		"scout":      func(a *ScoutAlarm) { a.ScoutID = "3f1c2a8e-0000-4000-8000-000000000002" },
	} {
		changed := base
		change(&changed)
		assert.NotEqual(t, base.Fingerprint(), changed.Fingerprint(), name)
	}
}
