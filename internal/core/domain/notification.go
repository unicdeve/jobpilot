package domain

import (
	"fmt"
	"time"
)

// NotificationKind is what a notification is about.
type NotificationKind string

// Notification kinds. Only scout runs notify (ADR-0028 §2).
const (
	// NotificationScoutFound is a run that created applications worth
	// telling about.
	NotificationScoutFound NotificationKind = "scout_found"
	// NotificationScoutFailed is a scheduled run that failed.
	NotificationScoutFailed NotificationKind = "scout_failed"
)

// Notification is something JobPilot told the user, or will (ADR-0028 §4).
//
// Its text says counts and a scout's name and nothing else, because a
// notification shows on a lock screen (ADR-0028 §3).
type Notification struct {
	ID      string           `json:"id"`
	Kind    NotificationKind `json:"kind"`
	Title   string           `json:"title"`
	Body    string           `json:"body"`
	ScoutID string           `json:"scout_id,omitempty"`
	RunID   string           `json:"run_id,omitempty"`
	// CreatedAt is when it was recorded, PostedAt when the operating
	// system was given it, and SeenAt when the user saw it in JobPilot.
	CreatedAt time.Time  `json:"created_at"`
	PostedAt  *time.Time `json:"posted_at,omitempty"`
	SeenAt    *time.Time `json:"seen_at,omitempty"`
}

// notificationPostWindow is how long a notification may wait to be posted.
// One about this morning, shown at dinner, misleads more than it tells; it
// waits in the app instead (ADR-0028 §4).
const notificationPostWindow = time.Hour

// Postable reports whether a notification should still be given to the
// operating system at now.
func (n Notification) Postable(now time.Time) bool {
	return n.PostedAt == nil && now.Sub(n.CreatedAt) <= notificationPostWindow
}

// NotificationSettings are the user's choices about notifications
// (ADR-0028 §6).
type NotificationSettings struct {
	// Enabled is whether to post notifications at all. Off stops posting,
	// not recording: the app still shows what the user has not seen.
	Enabled bool `json:"enabled"`
	// StrongMatchScore is the score an application's match needs to count
	// as a strong match.
	StrongMatchScore int `json:"strong_match_score"`
}

// DefaultNotificationSettings is what a user who has chosen nothing gets.
func DefaultNotificationSettings() NotificationSettings {
	return NotificationSettings{Enabled: true, StrongMatchScore: 80}
}

// Validate reports whether the settings can be kept.
func (s NotificationSettings) Validate() error {
	if s.StrongMatchScore < 1 || s.StrongMatchScore > 100 {
		return fmt.Errorf("a strong match must need a score between 1 and 100, got %d", s.StrongMatchScore)
	}
	return nil
}

// ScoutRunOutcome is what a finished scout run tells the notification
// rules: which scout, what started it, what it created, and whether it
// failed.
type ScoutRunOutcome struct {
	ScoutID   string
	ScoutName string
	RunID     string
	Trigger   RunTrigger
	Report    *ScoutRunReport
	Failed    bool
}

// ScoutRunNotification decides whether a finished scout run tells the
// user, and writes what it says (ADR-0028 §2, §3). It returns nil when the
// run says nothing.
//
//   - A scheduled run tells when it created applications or failed.
//   - Any run tells when it created an application for a strong match:
//     one at or above strongScore.
//   - A run tells once, however many things it found.
//
// Whether a score is strong is arithmetic; no model is asked.
func ScoutRunNotification(run ScoutRunOutcome, strongScore int) *Notification {
	created, strong := 0, 0
	if run.Report != nil {
		for _, verdict := range run.Report.Verdicts {
			if !verdict.Eligible {
				continue
			}
			created++
			if verdict.HasScore && verdict.Score >= strongScore {
				strong++
			}
		}
	}
	scheduled := run.Trigger == RunTriggerSchedule

	notification := &Notification{ScoutID: run.ScoutID, RunID: run.RunID}
	switch {
	case created > 0 && (scheduled || strong > 0):
		notification.Kind = NotificationScoutFound
		notification.Title = fmt.Sprintf("%s found %s for you", run.ScoutName, count(created, "job", "jobs"))
		notification.Body = foundBody(created, strong)
	case scheduled && run.Failed:
		notification.Kind = NotificationScoutFailed
		notification.Title = fmt.Sprintf("%s couldn’t run", run.ScoutName)
		notification.Body = "Open JobPilot to see why."
	default:
		return nil
	}
	return notification
}

// foundBody says how many of what a run created are strong matches, and
// that they wait for the user.
func foundBody(created, strong int) string {
	switch {
	case strong == 0:
		return readyForReview(created)
	case strong == created && created == 1:
		return "It’s a strong match, ready for review."
	case strong == created:
		return "All of them are strong matches, ready for review."
	case strong == 1:
		return "1 of them is a strong match. " + readyForReview(created)
	}
	return fmt.Sprintf("%d of them are strong matches. %s", strong, readyForReview(created))
}

func readyForReview(created int) string {
	if created == 1 {
		return "It’s ready for review."
	}
	return "They’re ready for review."
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
