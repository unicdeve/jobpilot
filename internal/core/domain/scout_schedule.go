package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ScheduleKind is how often a Scout runs on its own (ADR-0027 §1).
type ScheduleKind string

// Schedule kinds. A cron expression is deliberately not among them: it can
// express anything and be checked for nothing.
const (
	// ScheduleOff means nothing starts the Scout but the user.
	ScheduleOff ScheduleKind = "off"
	// ScheduleEvery runs the Scout every EveryHours hours.
	ScheduleEvery ScheduleKind = "every"
	// ScheduleDaily runs the Scout once a day at DailyAt.
	ScheduleDaily ScheduleKind = "daily"
)

// scheduleGrace is how early an alarm may fire and still count as the
// scheduled time: an alarm a few seconds early is not a time not yet
// reached (ADR-0027 §2).
const scheduleGrace = 5 * time.Minute

// ScoutSchedule is when a Scout runs on its own.
type ScoutSchedule struct {
	Kind ScheduleKind `json:"kind"`
	// EveryHours is the interval for ScheduleEvery, 1 to 24.
	EveryHours int `json:"every_hours,omitempty"`
	// DailyAt is the time of day for ScheduleDaily, in minutes after the
	// computer's local midnight. Local rather than the profile's time zone:
	// the profile says where the candidate works, the clock on the machine
	// says when they are at it.
	DailyAt int `json:"daily_at,omitempty"`
}

// IsScheduled reports whether anything but the user starts the Scout.
func (s ScoutSchedule) IsScheduled() bool {
	return s.Kind == ScheduleEvery || s.Kind == ScheduleDaily
}

// Normalized returns the schedule with an empty kind read as off, and the
// fields its kind does not use cleared, so two schedules that mean the same
// thing compare equal.
func (s ScoutSchedule) Normalized() ScoutSchedule {
	switch s.Kind {
	case ScheduleEvery:
		return ScoutSchedule{Kind: ScheduleEvery, EveryHours: s.EveryHours}
	case ScheduleDaily:
		return ScoutSchedule{Kind: ScheduleDaily, DailyAt: s.DailyAt}
	case "", ScheduleOff:
		return ScoutSchedule{Kind: ScheduleOff}
	}
	return s
}

// Validate reports whether the schedule can be kept, and says what is wrong.
func (s ScoutSchedule) Validate() error {
	switch s.Kind {
	case "", ScheduleOff:
		return nil
	case ScheduleEvery:
		if s.EveryHours < 1 || s.EveryHours > 24 {
			return fmt.Errorf("a scout can run every 1 to 24 hours, got %d", s.EveryHours)
		}
		return nil
	case ScheduleDaily:
		if s.DailyAt < 0 || s.DailyAt >= 24*60 {
			return fmt.Errorf("a daily time must be between 00:00 and 23:59, got minute %d", s.DailyAt)
		}
		return nil
	}
	return fmt.Errorf("unknown schedule %q", s.Kind)
}

// DueAt is the moment the Scout becomes due, given since: the later of when
// its last run started and when its schedule was set (ADR-0027 §2).
//
// Measured from a run's start, so "every 6 hours" does not drift by the
// length of each run. Measured from when the schedule was set, so a Scout
// scheduled at 10:00 for "daily at 08:00" is first due tomorrow, not the
// moment it is saved.
//
// It is meaningful only for a scheduled kind. A zero since, a Scout that
// has never run and has no record of when it was scheduled, is due at once.
//
// A daily time is read in since's time zone. Due and NextRun put since in
// the computer's first, since a time read back from the database may be
// in UTC.
func (s ScoutSchedule) DueAt(since time.Time) time.Time {
	if since.IsZero() {
		return since
	}
	switch s.Kind {
	case ScheduleEvery:
		return since.Add(time.Duration(s.EveryHours)*time.Hour - scheduleGrace)
	case ScheduleDaily:
		// The first daily time after since, past the grace an alarm is
		// allowed: a run that started at 07:59 because its 08:00 alarm
		// fired early has had today's.
		after := since.Add(scheduleGrace)
		at := dailyMoment(after, s.DailyAt)
		if !at.After(after) {
			at = dailyMoment(after.AddDate(0, 0, 1), s.DailyAt)
		}
		return at.Add(-scheduleGrace)
	}
	return time.Time{}
}

// Due reports whether the Scout should run now. now's time zone is the
// computer's, which a daily time is read in.
func (s ScoutSchedule) Due(now, since time.Time) bool {
	return s.IsScheduled() && !now.Before(s.DueAt(since.In(now.Location())))
}

// NextRun is when the operating system's alarm will next find the Scout
// due, for showing the user: the daily time itself, or the first whole hour
// at or after it falls due for "every N hours", whose alarm fires hourly.
// A Scout already due runs at its next alarm, so NextRun is never before
// now's next alarm.
func (s ScoutSchedule) NextRun(now, since time.Time) time.Time {
	due := s.DueAt(since.In(now.Location()))
	if due.Before(now) {
		due = now
	}
	switch s.Kind {
	case ScheduleEvery:
		hour := due.Truncate(time.Hour)
		if hour.Before(due) {
			hour = hour.Add(time.Hour)
		}
		return hour
	case ScheduleDaily:
		at := dailyMoment(due, s.DailyAt)
		if at.Before(due) {
			at = dailyMoment(due.AddDate(0, 0, 1), s.DailyAt)
		}
		return at
	}
	return time.Time{}
}

// dailyMoment is minutes after midnight on day's date, in day's location.
// It is built from the date rather than added to midnight, so a day that
// is 23 or 25 hours long still gives the wall-clock time asked for.
func dailyMoment(day time.Time, minutes int) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, minutes/60, minutes%60, 0, 0, day.Location())
}

// ScoutAlarm is the operating-system entry a scheduled Scout needs: what to
// start, and when (ADR-0027 §6). The database is the schedule; an alarm is
// derived from it and never read back as a source of truth.
type ScoutAlarm struct {
	ScoutID  string
	Schedule ScoutSchedule
	// Executable is the daemon binary the alarm starts, with
	// `scout wake <id>`.
	Executable string
	// Env is the environment the alarm sets, so the daemon it starts finds
	// the same data directory as the one that wrote it.
	Env map[string]string
}

// alarmFormat changes whenever what an alarm says changes, so every entry
// written before is rewritten. 2: an alarm runs a script that removes it
// once JobPilot is gone (ADR-0029 §5).
const alarmFormat = "2"

// Fingerprint identifies what an alarm says. Two alarms with the same
// fingerprint are the same entry, and one with a different fingerprint is
// rewritten.
func (a ScoutAlarm) Fingerprint() string {
	var b strings.Builder
	fmt.Fprintf(&b, "format=%s\nscout=%s\nkind=%s\nevery=%d\ndaily=%d\nexe=%s\n",
		alarmFormat, a.ScoutID, a.Schedule.Kind, a.Schedule.EveryHours, a.Schedule.DailyAt, a.Executable)
	keys := make([]string, 0, len(a.Env))
	for key := range a.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "env %s=%s\n", key, a.Env[key])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

// ScoutAlarmEntry is one alarm the operating system holds for JobPilot.
type ScoutAlarmEntry struct {
	ScoutID     string
	Fingerprint string
	// SwitchedOff is an entry the user turned off outside JobPilot, as
	// macOS lets them in Login Items. JobPilot reports it and does not
	// turn it back on behind their back.
	SwitchedOff bool
}

// AlarmState is whether a Scout's schedule is in the operating system.
type AlarmState string

// Alarm states, shown on a scheduled Scout.
const (
	// AlarmScheduled is an alarm in place and on.
	AlarmScheduled AlarmState = "scheduled"
	// AlarmMissing is an alarm that should exist and does not, with why.
	AlarmMissing AlarmState = "missing"
	// AlarmSwitchedOff is an alarm the user turned off outside JobPilot.
	AlarmSwitchedOff AlarmState = "switched_off"
	// AlarmUnavailable is a system with no scheduler JobPilot can use.
	AlarmUnavailable AlarmState = "unavailable"
)

// ScoutAlarmStatus is what the operating system holds for one scheduled
// Scout, for the user to see.
type ScoutAlarmStatus struct {
	State  AlarmState `json:"state"`
	Reason string     `json:"reason,omitempty"`
}

// ScoutActivity is what a Scout is doing right now.
type ScoutActivity string

// Scout activities.
const (
	// ScoutRunning is a Scout with a run in progress.
	ScoutRunning ScoutActivity = "running"
	// ScoutWaiting is a Scout whose run waits for another Scout's to
	// finish: runs happen one at a time (ADR-0027 §5).
	ScoutWaiting ScoutActivity = "waiting"
)

// WakeOutcome is what the daemon did when a Scout's alarm fired.
type WakeOutcome string

// Wake outcomes.
const (
	// WakeStarted is a due Scout whose run has begun.
	WakeStarted WakeOutcome = "started"
	// WakeNotDue is a Scout whose time has not come.
	WakeNotDue WakeOutcome = "not_due"
	// WakeDisabled is a paused Scout.
	WakeDisabled WakeOutcome = "disabled"
	// WakeUnscheduled is a Scout whose schedule was turned off.
	WakeUnscheduled WakeOutcome = "unscheduled"
	// WakeAlreadyRunning is a Scout already running or waiting to.
	WakeAlreadyRunning WakeOutcome = "already_running"
)

// ScoutWake is the daemon's answer to an alarm.
type ScoutWake struct {
	Outcome   WakeOutcome `json:"outcome"`
	NextRunAt *time.Time  `json:"next_run_at,omitempty"`
}
