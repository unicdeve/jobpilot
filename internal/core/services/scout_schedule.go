package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// errNoLongerDue ends a scheduled run that waited its turn and found a
// manual run had gone in the meantime. It is not a failure, and no run is
// recorded for it.
var errNoLongerDue = errors.New("no longer due")

// AlarmTarget is what every alarm starts: the daemon's own executable, and
// the environment that makes it find the same data directory as the daemon
// that wrote the alarm (ADR-0027 §4).
type AlarmTarget struct {
	Executable string
	Env        map[string]string
}

// alarmTimeout bounds one reconcile. launchctl, systemctl and schtasks are
// other programs, and the keystore taught that a platform integration can
// hang rather than fail (ADR-0019).
const alarmTimeout = 30 * time.Second

// alarmStatusTTL is how long what the operating system said is trusted
// before a read asks again. The user can remove or switch off an alarm
// outside JobPilot at any time.
const alarmStatusTTL = time.Minute

// WithAlarms gives the service the operating system's scheduler, and what
// every alarm starts. Without it, a schedule is kept but nothing starts it.
func (s *ScoutService) WithAlarms(alarms ports.ScoutAlarms, target AlarmTarget) *ScoutService {
	s.alarms, s.alarmTarget = alarms, target
	return s
}

// Start is called once when the daemon starts, before it serves anything.
// Scheduled runs belong to ctx, which ends when the daemon stops.
//
// The alarms are then made to match the database, in the background so a
// slow scheduler does not hold up the daemon becoming reachable: an alarm
// the user removed is put back, and one whose executable moved, because
// JobPilot was moved or rebuilt, is rewritten.
func (s *ScoutService) Start(ctx context.Context) {
	s.background = ctx
	s.scheduled.Add(1)
	go func() {
		defer s.scheduled.Done()
		if err := s.Reconcile(ctx); err != nil {
			slog.WarnContext(ctx, "failed to bring scout alarms up to date", "error", err)
		}
	}()
}

// Wait blocks until every scheduled run has finished recording itself, and
// the start-up reconcile has finished, so the daemon does not close the
// database under either.
func (s *ScoutService) Wait() {
	s.scheduled.Wait()
}

// Wake is what an alarm asks: run this Scout if it is due (ADR-0027 §2).
//
// The alarm only asks. Whether a Scout runs is decided here, so an alarm
// that fires twice, early, or for a Scout changed since it was written does
// nothing rather than something wrong. A due Scout's run is started in the
// background and Wake returns at once: the run belongs to the daemon, and
// no operating system's limit on how long a task may take applies to it.
func (s *ScoutService) Wake(ctx context.Context, scoutID string) (*domain.ScoutWake, error) {
	scout, err := s.scouts.GetByID(ctx, scoutID)
	if errors.Is(err, ports.ErrNotFound) {
		// An alarm for a Scout that is gone is an alarm nobody removed.
		go s.reconcileInBackground()
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if !scout.IsEnabled {
		return &domain.ScoutWake{Outcome: domain.WakeDisabled}, nil
	}
	if !scout.Schedule.IsScheduled() {
		return &domain.ScoutWake{Outcome: domain.WakeUnscheduled}, nil
	}

	since, err := s.since(ctx, scout)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if !scout.Schedule.Due(now, since) {
		next := scout.Schedule.NextRun(now, since)
		return &domain.ScoutWake{Outcome: domain.WakeNotDue, NextRunAt: &next}, nil
	}

	// Checked and claimed under one lock, so two alarms arriving together
	// start one run.
	s.mu.Lock()
	busy := s.running == scout.ID || s.waiting[scout.ID] > 0
	if !busy {
		s.waiting[scout.ID]++
	}
	s.mu.Unlock()
	if busy {
		return &domain.ScoutWake{Outcome: domain.WakeAlreadyRunning}, nil
	}

	s.scheduled.Add(1)
	go func() {
		defer s.scheduled.Done()
		s.runScheduled(scout.ID)
	}()
	return &domain.ScoutWake{Outcome: domain.WakeStarted}, nil
}

// runScheduled runs a Scout its alarm found due. Its failures are in its
// run record, as a manual run's are, and in the daemon's log.
func (s *ScoutService) runScheduled(scoutID string) {
	ctx := s.background
	release, err := s.acquire(ctx, scoutID)
	if err != nil {
		return
	}
	defer release()

	report, err := s.execute(ctx, scoutID, domain.RunTriggerSchedule)
	switch {
	case errors.Is(err, errNoLongerDue):
	case err != nil:
		slog.WarnContext(ctx, "scheduled scout run failed", "scout_id", scoutID, "error", err)
	default:
		slog.InfoContext(ctx, "scheduled scout run finished", "scout_id", scoutID,
			"considered", report.Considered, "created", report.Created)
	}
}

// enqueue records that a run of the Scout is waiting for its turn. It is
// separate from acquire so that Wake can claim the place before the run's
// goroutine starts.
func (s *ScoutService) enqueue(scoutID string) {
	s.mu.Lock()
	s.waiting[scoutID]++
	s.mu.Unlock()
}

// acquire waits for the run lock on behalf of a run enqueue recorded, and
// returns the function that gives it back.
func (s *ScoutService) acquire(ctx context.Context, scoutID string) (func(), error) {
	dequeue := func() {
		s.waiting[scoutID]--
		if s.waiting[scoutID] <= 0 {
			delete(s.waiting, scoutID)
		}
	}
	select {
	case s.runLock <- struct{}{}:
	case <-ctx.Done():
		s.mu.Lock()
		dequeue()
		s.mu.Unlock()
		return nil, ctx.Err()
	}

	s.mu.Lock()
	dequeue()
	s.running = scoutID
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.running = ""
		s.mu.Unlock()
		<-s.runLock
	}, nil
}

// activity is what the Scout is doing right now.
func (s *ScoutService) activity(scoutID string) domain.ScoutActivity {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.running == scoutID:
		return domain.ScoutRunning
	case s.waiting[scoutID] > 0:
		return domain.ScoutWaiting
	}
	return ""
}

// since is what a Scout's next scheduled run is measured from: the later
// of when its last run started and when its schedule was set.
func (s *ScoutService) since(ctx context.Context, scout *domain.Scout) (time.Time, error) {
	var since time.Time
	if scout.ScheduleSetAt != nil {
		since = *scout.ScheduleSetAt
	}
	last, err := s.runs.LastScoutStart(ctx, scout.ID)
	if err != nil {
		return time.Time{}, err
	}
	if last != nil && last.After(since) {
		since = *last
	}
	return since, nil
}

// describe fills in what is worked out rather than stored: when the Scout
// next runs, whether the operating system holds its schedule, and what it
// is doing now.
func (s *ScoutService) describe(ctx context.Context, scout *domain.Scout) {
	scout.Activity = s.activity(scout.ID)
	scout.NextRunAt, scout.Alarm = nil, nil
	if !scout.IsEnabled || !scout.Schedule.IsScheduled() {
		return
	}

	since, err := s.since(ctx, scout)
	if err != nil {
		slog.WarnContext(ctx, "failed to work out a scout's next run", "scout_id", scout.ID, "error", err)
	} else {
		next := scout.Schedule.NextRun(s.now(), since)
		scout.NextRunAt = &next
	}

	s.alarmMu.Lock()
	status, ok := s.alarmStatus[scout.ID]
	s.alarmMu.Unlock()
	switch {
	case s.alarms == nil:
		status = domain.ScoutAlarmStatus{State: domain.AlarmUnavailable, Reason: ports.ErrAlarmsUnavailable.Error()}
	case !ok:
		status = domain.ScoutAlarmStatus{State: domain.AlarmMissing, Reason: "not yet written"}
	}
	scout.Alarm = &status
}

// scoutsChanged brings the alarms into line after a Scout was created,
// changed or deleted. Its failure is not the save's: the Scout is saved,
// and shows that it is not scheduled, and why, until a later reconcile
// succeeds (ADR-0027 §6).
func (s *ScoutService) scoutsChanged(ctx context.Context) {
	if err := s.Reconcile(ctx); err != nil {
		slog.WarnContext(ctx, "failed to bring scout alarms up to date", "error", err)
	}
}

// ScoutsChanged is scoutsChanged for a change made elsewhere: deleting a
// saved search deletes the Scouts that run it.
func (s *ScoutService) ScoutsChanged(ctx context.Context) {
	s.scoutsChanged(ctx)
}

func (s *ScoutService) reconcileInBackground() {
	s.scoutsChanged(context.WithoutCancel(s.background))
}

// refreshAlarmsIfStale reconciles when what the operating system last said
// is older than alarmStatusTTL, so an alarm switched off or removed outside
// JobPilot is shown, or put back, without a restart.
func (s *ScoutService) refreshAlarmsIfStale(ctx context.Context) {
	if s.alarms == nil {
		return
	}
	s.alarmMu.Lock()
	stale := s.now().Sub(s.alarmsRead) > alarmStatusTTL
	s.alarmMu.Unlock()
	if stale {
		s.scoutsChanged(ctx)
	}
}

// Reconcile makes the operating system's alarms match the database, which
// is the only record of what is scheduled (ADR-0027 §6). Each enabled,
// scheduled Scout gets an alarm, an alarm that says something different
// is rewritten, and every other alarm JobPilot holds is removed.
//
// A failure to write one alarm does not stop the rest. Each Scout keeps
// its own status, which the user sees on it.
func (s *ScoutService) Reconcile(ctx context.Context) error {
	if s.alarms == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), alarmTimeout)
	defer cancel()
	s.alarmMu.Lock()
	defer s.alarmMu.Unlock()

	scouts, err := s.scouts.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to read scouts: %w", err)
	}
	desired := map[string]domain.ScoutAlarm{}
	for _, scout := range scouts {
		if scout.IsEnabled && scout.Schedule.IsScheduled() {
			desired[scout.ID] = domain.ScoutAlarm{
				ScoutID:    scout.ID,
				Schedule:   scout.Schedule.Normalized(),
				Executable: s.alarmTarget.Executable,
				Env:        s.alarmTarget.Env,
			}
		}
	}

	status := make(map[string]domain.ScoutAlarmStatus, len(desired))
	entries, err := s.alarms.List(ctx)
	if err != nil {
		state := domain.AlarmMissing
		if errors.Is(err, ports.ErrAlarmsUnavailable) {
			state = domain.AlarmUnavailable
		}
		for id := range desired {
			status[id] = domain.ScoutAlarmStatus{State: state, Reason: err.Error()}
		}
		s.alarmStatus, s.alarmsRead = status, s.now()
		if state == domain.AlarmUnavailable {
			return nil
		}
		return fmt.Errorf("failed to read the scheduled scouts from the operating system: %w", err)
	}

	existing := make(map[string]domain.ScoutAlarmEntry, len(entries))
	for _, entry := range entries {
		existing[entry.ScoutID] = entry
	}

	var errs []error
	for id, alarm := range desired {
		entry, found := existing[id]
		if !found || entry.Fingerprint != alarm.Fingerprint() {
			if err := s.alarms.Write(ctx, alarm); err != nil {
				status[id] = domain.ScoutAlarmStatus{State: domain.AlarmMissing, Reason: err.Error()}
				errs = append(errs, fmt.Errorf("scout %s: %w", id, err))
				continue
			}
		}
		// Write keeps an alarm the user switched off switched off, so a
		// rewritten one is still theirs to turn back on.
		if found && entry.SwitchedOff {
			status[id] = domain.ScoutAlarmStatus{State: domain.AlarmSwitchedOff,
				Reason: "switched off outside JobPilot"}
			continue
		}
		status[id] = domain.ScoutAlarmStatus{State: domain.AlarmScheduled}
	}
	for id := range existing {
		if _, keep := desired[id]; keep {
			continue
		}
		if err := s.alarms.Remove(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("removing the alarm for scout %s: %w", id, err))
		}
	}

	s.alarmStatus, s.alarmsRead = status, s.now()
	return errors.Join(errs...)
}

// DescribeAlarms says what alarms JobPilot holds, for "delete all data" to
// list among what it removes outside the database, or "" when there are
// none or no scheduler to ask.
func (s *ScoutService) DescribeAlarms(ctx context.Context) string {
	if s.alarms == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, alarmTimeout)
	defer cancel()
	entries, err := s.alarms.List(ctx)
	if err != nil || len(entries) == 0 {
		return ""
	}
	return fmt.Sprintf("%d scheduled %s in the operating system's scheduler",
		len(entries), plural(len(entries), "scout", "scouts"))
}

// RemoveAllAlarms removes every alarm JobPilot holds. "Delete all data"
// calls it before the database, because an alarm left behind would fire
// for a Scout that no longer exists, forever. Should the deletion stop
// after this, the daemon's next reconcile puts back the alarms of the
// Scouts that remain.
func (s *ScoutService) RemoveAllAlarms(ctx context.Context) error {
	if s.alarms == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, alarmTimeout)
	defer cancel()
	s.alarmMu.Lock()
	defer s.alarmMu.Unlock()

	entries, err := s.alarms.List(ctx)
	if errors.Is(err, ports.ErrAlarmsUnavailable) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if err := s.alarms.Remove(ctx, entry.ScoutID); err != nil {
			errs = append(errs, err)
		}
	}
	s.alarmStatus, s.alarmsRead = nil, time.Time{}
	return errors.Join(errs...)
}
