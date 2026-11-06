package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// ScoutTaskType labels a scout run in the agent trail.
const ScoutTaskType = domain.AgentTaskScoutRun

// ScoutJobFinder is the part of job discovery a Scout uses.
//
// It is narrowed to these two calls on purpose. A Scout must not be able
// to reach the browser — ADR-0014 makes that a boundary rather than a
// setting — and the way to make a boundary hold is to withhold the
// capability, not to remember not to use it. JobDiscoveryService.ImportJob
// can open a browser; it is not on this interface.
type ScoutJobFinder interface {
	Search(ctx context.Context, criteria domain.SearchCriteria) (*domain.DiscoveryResult, error)
	ListJobs(ctx context.Context, filter domain.JobFilter, limit, offset int) (*domain.JobPage, error)
}

// ScoutMatcher scores a job against a candidate.
type ScoutMatcher interface {
	AnalyzeJobForCandidate(ctx context.Context, candidateID, jobID string) (*domain.JobMatch, error)
}

// ScoutService runs Scouts: a saved search, judged against the candidate,
// turned into applications waiting for review (ADR-0014, ADR-0018).
//
// It creates applications. It never submits one, never opens a browser and
// never contacts an employer.
type ScoutService struct {
	scouts        ports.ScoutRepository
	savedSearches ports.SavedSearchRepository
	candidates    ports.CandidateRepository
	applications  ports.ApplicationRepository
	runs          ports.AgentRunRepository
	events        ports.AgentEventRepository
	finder        ScoutJobFinder
	matcher       ScoutMatcher
	// now is injectable so a test can assert on recorded times.
	now func() time.Time

	// runLock holds one Scout run at a time, whatever started it
	// (ADR-0027 §5). It is a channel rather than a mutex so that a run
	// waiting for it can give up when its request is cancelled.
	runLock chan struct{}
	// mu guards running and waiting, which say what each Scout is doing.
	mu      sync.Mutex
	running string
	waiting map[string]int

	// background is the daemon's lifetime, which a scheduled run belongs
	// to rather than to the request that woke it (ADR-0027 §4).
	background context.Context
	scheduled  sync.WaitGroup

	// alarms is the operating system's scheduler, or nil where there is
	// none to use (ADR-0027 §6).
	alarms      ports.ScoutAlarms
	alarmTarget AlarmTarget
	alarmMu     sync.Mutex
	alarmStatus map[string]domain.ScoutAlarmStatus
	alarmsRead  time.Time

	// notifier is told how every run ended, and decides whether that is
	// worth telling the user (ADR-0028). Nil tells no one.
	notifier ScoutRunNotifier
}

// ScoutRunNotifier is told how a scout run ended.
type ScoutRunNotifier interface {
	ScoutRunFinished(ctx context.Context, run domain.ScoutRunOutcome)
}

// WithNotifier has the service tell notifier how every run ends.
func (s *ScoutService) WithNotifier(notifier ScoutRunNotifier) *ScoutService {
	s.notifier = notifier
	return s
}

// NewScoutService constructs a ScoutService.
func NewScoutService(
	scouts ports.ScoutRepository,
	savedSearches ports.SavedSearchRepository,
	candidates ports.CandidateRepository,
	applications ports.ApplicationRepository,
	runs ports.AgentRunRepository,
	events ports.AgentEventRepository,
	finder ScoutJobFinder,
	matcher ScoutMatcher,
) *ScoutService {
	return &ScoutService{
		scouts: scouts, savedSearches: savedSearches, candidates: candidates,
		applications: applications, runs: runs, events: events,
		finder: finder, matcher: matcher, now: time.Now,
		runLock:    make(chan struct{}, 1),
		waiting:    map[string]int{},
		background: context.Background(),
	}
}

// maxJobsConsideredPerRun bounds how many jobs one run will score.
//
// Scoring calls the model once per job, so an unbounded run against a
// large local history is a long wait with no way to see progress. The
// ceiling on applications created does not bound this: a run can reject
// every job it looks at.
const maxJobsConsideredPerRun = 200

// CreateScout validates and stores a new Scout.
func (s *ScoutService) CreateScout(ctx context.Context, scout *domain.Scout) error {
	if scout.Source == "" {
		scout.Source = domain.ScoutSourceLocal
	}
	if scout.MaxApplicationsPerRun == 0 {
		scout.MaxApplicationsPerRun = 10
	}
	scout.Schedule = scout.Schedule.Normalized()
	if err := scout.Validate(); err != nil {
		return err
	}
	if _, err := s.savedSearches.GetByID(ctx, scout.SavedSearchID); err != nil {
		return fmt.Errorf("the scout's saved search could not be found: %w", ports.AsReference(err))
	}
	scout.ScheduleSetAt = nil
	if scout.Schedule.IsScheduled() {
		now := s.now()
		scout.ScheduleSetAt = &now
	}
	if err := s.scouts.Save(ctx, scout); err != nil {
		return err
	}
	s.scoutsChanged(ctx)
	s.describe(ctx, scout)
	return nil
}

// UpdateScout validates and persists changes to a Scout.
//
// What the user cannot change is carried over from the stored Scout: when
// it last ran, and when its schedule was set unless the schedule is what
// changed.
func (s *ScoutService) UpdateScout(ctx context.Context, scout *domain.Scout) error {
	scout.Schedule = scout.Schedule.Normalized()
	if err := scout.Validate(); err != nil {
		return err
	}
	if _, err := s.savedSearches.GetByID(ctx, scout.SavedSearchID); err != nil {
		return fmt.Errorf("the scout's saved search could not be found: %w", ports.AsReference(err))
	}
	stored, err := s.scouts.GetByID(ctx, scout.ID)
	if err != nil {
		return err
	}

	scout.LastRunAt, scout.CreatedAt = stored.LastRunAt, stored.CreatedAt
	scout.ScheduleSetAt = stored.ScheduleSetAt
	switch {
	case !scout.Schedule.IsScheduled():
		scout.ScheduleSetAt = nil
	case scout.Schedule != stored.Schedule.Normalized() || scout.ScheduleSetAt == nil:
		now := s.now()
		scout.ScheduleSetAt = &now
	}
	if err := s.scouts.Update(ctx, scout); err != nil {
		return err
	}
	s.scoutsChanged(ctx)
	s.describe(ctx, scout)
	return nil
}

// ListScouts returns every Scout, with when each next runs, whether the
// operating system holds its schedule, and what it is doing now.
func (s *ScoutService) ListScouts(ctx context.Context) ([]*domain.Scout, error) {
	scouts, err := s.scouts.List(ctx)
	if err != nil {
		return nil, err
	}
	s.refreshAlarmsIfStale(ctx)
	for _, scout := range scouts {
		s.describe(ctx, scout)
	}
	return scouts, nil
}

// GetScout returns one Scout, described as ListScouts describes it.
func (s *ScoutService) GetScout(ctx context.Context, id string) (*domain.Scout, error) {
	scout, err := s.scouts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.refreshAlarmsIfStale(ctx)
	s.describe(ctx, scout)
	return scout, nil
}

// DeleteScout removes a Scout, and its alarm with it.
func (s *ScoutService) DeleteScout(ctx context.Context, id string) error {
	if err := s.scouts.Delete(ctx, id); err != nil {
		return err
	}
	s.scoutsChanged(ctx)
	return nil
}

// ListRuns returns recent scout runs, newest first.
func (s *ScoutService) ListRuns(ctx context.Context, limit int) ([]*domain.AgentRun, error) {
	return s.runs.List(ctx, limit)
}

// ListRunEvents returns what one run did, in order.
func (s *ScoutService) ListRunEvents(ctx context.Context, runID string) ([]*domain.AgentEvent, error) {
	return s.events.ListByRun(ctx, runID)
}

// Run executes a Scout once, now, because the user asked.
//
// A run that fails partway keeps the applications it already created and
// records the failure (AC-006). Nothing here is transactional across jobs
// on purpose: an application already created is a real thing the user can
// act on, and undoing it because a later job failed to score would throw
// away good work for tidiness.
//
// Runs happen one at a time, so this waits for any run in progress, of
// this Scout or another, before it starts (ADR-0027 §5).
func (s *ScoutService) Run(ctx context.Context, scoutID string) (*domain.ScoutRunReport, error) {
	scout, err := s.scouts.GetByID(ctx, scoutID)
	if err != nil {
		return nil, fmt.Errorf("failed to load the scout: %w", err)
	}
	if !scout.IsEnabled {
		return nil, fmt.Errorf("scout %q is disabled", scout.Name)
	}

	s.enqueue(scout.ID)
	release, err := s.acquire(ctx, scout.ID)
	if err != nil {
		return nil, fmt.Errorf("stopped waiting for another scout's run to finish: %w", err)
	}
	defer release()
	return s.execute(ctx, scout.ID, domain.RunTriggerManual)
}

// execute runs a Scout, holding the run lock. It reads the Scout again,
// because one that waited may have been changed or paused meanwhile.
//
// However the run ends, the notifier is told, once the Scout is known: a
// scheduled run that could not even start is worth telling about too.
func (s *ScoutService) execute(ctx context.Context, scoutID string, trigger domain.RunTrigger) (report *domain.ScoutRunReport, runErr error) {
	scout, err := s.scouts.GetByID(ctx, scoutID)
	if err != nil {
		return nil, fmt.Errorf("failed to load the scout: %w", err)
	}
	if !scout.IsEnabled {
		if trigger == domain.RunTriggerSchedule {
			// Paused while it waited its turn.
			return nil, errNoLongerDue
		}
		return nil, fmt.Errorf("scout %q is disabled", scout.Name)
	}
	defer func() {
		if s.notifier == nil || errors.Is(runErr, errNoLongerDue) {
			return
		}
		outcome := domain.ScoutRunOutcome{
			ScoutID: scout.ID, ScoutName: scout.Name, Trigger: trigger,
			Report: report, Failed: runErr != nil,
		}
		if report != nil {
			outcome.RunID = report.RunID
		}
		s.notifier.ScoutRunFinished(ctx, outcome)
	}()
	if trigger == domain.RunTriggerSchedule {
		// A manual run may have gone while this one waited its turn,
		// which makes this one not due any more.
		since, err := s.since(ctx, scout)
		if err != nil {
			return nil, err
		}
		if !scout.Schedule.Due(s.now(), since) {
			return nil, errNoLongerDue
		}
	}

	// The run is recorded before anything that can fail, so a failure is
	// a run like any other: it shows in Activity, and a scheduled scout
	// that cannot start is not due again until its next time, rather than
	// at every alarm (ADR-0027 §2).
	run := &domain.AgentRun{
		// No ApplicationID: a scout run creates several applications and
		// belongs to none of them (ADR-0018 §4).
		ScoutID:   scout.ID,
		Trigger:   trigger,
		TaskType:  ScoutTaskType,
		Status:    domain.AgentRunStatusRunning,
		StartedAt: s.now(),
	}
	if err := s.runs.Save(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to record the scout run: %w", err)
	}
	report = &domain.ScoutRunReport{ScoutID: scout.ID, RunID: run.ID}

	runErr = s.start(ctx, scout, run.ID, report)
	if runErr == nil {
		runErr = ctx.Err()
	}

	s.finish(ctx, run, report, runErr)
	if runErr != nil {
		report.Error = runErr.Error()
	}
	return report, runErr
}

// start loads what a run needs and runs it.
func (s *ScoutService) start(ctx context.Context, scout *domain.Scout, runID string, report *domain.ScoutRunReport) error {
	search, err := s.savedSearches.GetByID(ctx, scout.SavedSearchID)
	if err != nil {
		return fmt.Errorf("failed to load the scout's saved search: %w", err)
	}
	candidate, err := s.candidates.GetFirstCandidate(ctx)
	if err != nil {
		return fmt.Errorf("failed to load the candidate profile: %w", err)
	}
	s.record(ctx, runID, "scout_started", "ok",
		fmt.Sprintf("scout %q, search %q", scout.Name, search.Name),
		fmt.Sprintf("floor %d, ceiling %d, source %s",
			scout.MinMatchScore, scout.MaxApplicationsPerRun, scout.Source))
	return s.considerJobs(ctx, scout, search, candidate, runID, report)
}

// considerJobs is the body of a run, split out so that however it ends the
// run record and the report are closed off the same way.
func (s *ScoutService) considerJobs(
	ctx context.Context,
	scout *domain.Scout,
	search *domain.SavedSearch,
	candidate *domain.Candidate,
	runID string,
	report *domain.ScoutRunReport,
) error {
	jobs, err := s.collectJobs(ctx, scout, search, runID)
	if err != nil {
		return err
	}

	applied, err := s.jobsAlreadyApplied(ctx, candidate.ID)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		// The ceiling is checked here as well as inside Eligible. A run
		// that has made its quota should stop looking, not keep scoring
		// jobs it has already decided it cannot act on (AC-005).
		if report.Created >= scout.MaxApplicationsPerRun {
			report.CeilingHit = true
			s.recordDecision(ctx, runID, "ceiling_reached", domain.PolicyDenied, "",
				fmt.Sprintf("stopped after creating %d applications", report.Created))
			return nil
		}

		report.Considered++
		verdict := domain.ScoutVerdict{JobID: job.ID, JobTitle: job.Title, Considered: true}
		input := domain.EligibilityInput{
			Job:            job,
			Criteria:       search.Criteria,
			AlreadyApplied: applied[job.ID],
			CreatedSoFar:   report.Created,
			Seeker:         candidate.Seeker(),
		}

		if ok, reason := scout.PreScreen(input); !ok {
			verdict.Reason = reason
			report.Verdicts = append(report.Verdicts, verdict)
			s.recordDecision(ctx, runID, "job_rejected", domain.PolicyDenied, job.Title, reason)
			continue
		}

		match, err := s.matcher.AnalyzeJobForCandidate(ctx, candidate.ID, job.ID)
		if cancelErr := ctx.Err(); cancelErr != nil {
			verdict.Reason = cancelErr.Error()
			report.Verdicts = append(report.Verdicts, verdict)
			return cancelErr
		}
		if err != nil {
			// One job that cannot be scored does not end the run: the
			// rest are still worth considering, and the failure is
			// recorded rather than swallowed.
			verdict.Reason = fmt.Sprintf("could not be scored: %v", err)
			report.Verdicts = append(report.Verdicts, verdict)
			s.record(ctx, runID, "job_scoring_failed", "error", job.Title, err.Error())
			continue
		}

		input.Score, input.HasScore = match.Score, true
		verdict.Score, verdict.HasScore = match.Score, true

		eligible, reason := scout.Eligible(input)
		verdict.Eligible, verdict.Reason = eligible, reason
		if !eligible {
			report.Verdicts = append(report.Verdicts, verdict)
			s.recordDecision(ctx, runID, "job_rejected", domain.PolicyDenied, job.Title, reason)
			continue
		}

		// Review mode, explicitly: a scout's output is something for the
		// user to look at, and no mode would submit it.
		application, err := s.createApplication(ctx, job.ID, candidate.ID)
		if err != nil {
			verdict.Eligible = false
			verdict.Reason = fmt.Sprintf("could not create an application: %v", err)
			report.Verdicts = append(report.Verdicts, verdict)
			s.record(ctx, runID, "application_failed", "error", job.Title, err.Error())
			return fmt.Errorf("failed to create an application for %q: %w", job.Title, err)
		}

		report.Created++
		applied[job.ID] = true
		report.Verdicts = append(report.Verdicts, verdict)
		s.recordDecision(ctx, runID, "application_created", domain.PolicyAllowed, job.Title,
			fmt.Sprintf("application %s, %s", application.ID, reason))
	}

	return nil
}

// collectJobs gathers the jobs a run will consider.
func (s *ScoutService) collectJobs(
	ctx context.Context, scout *domain.Scout, search *domain.SavedSearch, runID string,
) ([]*domain.Job, error) {
	if scout.Source == domain.ScoutSourceFetch {
		// Search stores what it finds, so the listing below picks the new
		// jobs up along with everything already known.
		found, err := s.finder.Search(ctx, search.Criteria)
		if err != nil {
			return nil, fmt.Errorf("failed to search the job sources: %w", err)
		}
		// The search has now run, as it would have from Discover, so the
		// desktop does not run it again on opening (AC-009 in
		// docs/02_features/04_job_discovery.md). Failing to record that
		// costs one repeated search, not the run.
		if err := s.savedSearches.MarkRun(ctx, search.ID, s.now()); err != nil {
			s.record(ctx, runID, "search_run_not_recorded", "error", search.Name, err.Error())
		}
		// A source that did not answer is its own step, so a run that
		// considered fewer jobs than usual says why.
		unanswered := found.Unanswered()
		for _, source := range unanswered {
			action, output := "source_failed", "answered with an error"
			if source.Status == domain.SourceStatusUnreachable {
				action, output = "source_unreachable", "could not be reached"
			}
			s.record(ctx, runID, action, "error", source.SourceName, output)
		}
		s.record(ctx, runID, "sources_searched", "ok", search.Name,
			fmt.Sprintf("%d jobs returned, %d new; %d of %d sources answered",
				len(found.Jobs), found.New, len(found.Sources)-len(unanswered), len(found.Sources)))
	}

	// Jobs the user ruled out are read too, so each is passed over with
	// its reason in the trail rather than silently absent from it.
	page, err := s.finder.ListJobs(ctx, domain.JobFilter{Interest: domain.JobsAll}, maxJobsConsideredPerRun, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read stored jobs: %w", err)
	}
	s.record(ctx, runID, "jobs_collected", "ok", string(scout.Source),
		fmt.Sprintf("%d jobs to consider", len(page.Jobs)))
	return page.Jobs, nil
}

// jobsAlreadyApplied is the set of jobs the candidate already has an
// application for, so a run does not offer the same job twice.
func (s *ScoutService) jobsAlreadyApplied(ctx context.Context, candidateID string) (map[string]bool, error) {
	existing, err := s.applications.ListByCandidate(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to read existing applications: %w", err)
	}
	applied := make(map[string]bool, len(existing))
	for _, application := range existing {
		applied[application.JobID] = true
	}
	return applied, nil
}

// createApplication makes the reviewable application a scout produces.
func (s *ScoutService) createApplication(ctx context.Context, jobID, candidateID string) (*domain.Application, error) {
	application := &domain.Application{
		JobID:       jobID,
		CandidateID: candidateID,
		State:       domain.ApplicationStateDiscovered,
	}
	if err := s.applications.Save(ctx, application); err != nil {
		return nil, err
	}
	return application, nil
}

// finish closes off the run record and the scout's last-run time.
func (s *ScoutService) finish(ctx context.Context, run *domain.AgentRun, report *domain.ScoutRunReport, runErr error) {
	// Persist the outcome even when the request was cancelled, but bound cleanup.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	completed := s.now()
	run.CompletedAt = &completed
	run.Status = domain.AgentRunStatusCompleted
	if runErr != nil {
		run.Status = domain.AgentRunStatusFailed
		run.Error = runErr.Error()
	}

	// A failure to write the trail must not discard the report: the
	// applications are already created and the user can see them.
	if err := s.runs.Update(ctx, run); err != nil {
		slog.WarnContext(ctx, "failed to finalize scout run", "run_id", run.ID, "error", err)
	}
	s.record(ctx, run.ID, "scout_finished", string(run.Status),
		fmt.Sprintf("%d jobs considered", report.Considered),
		fmt.Sprintf("%d applications created", report.Created))

	if err := s.scouts.MarkRun(ctx, report.ScoutID, completed); err != nil {
		slog.WarnContext(ctx, "failed to record scout last run", "scout_id", report.ScoutID, "error", err)
	}
}

// record appends one step to the run's trail.
//
// Trail writes never fail a run. The run's real output is the
// applications, and losing a line of history is not a reason to throw
// them away.
func (s *ScoutService) record(ctx context.Context, runID, action, status, input, output string) {
	s.save(ctx, &domain.AgentEvent{
		RunID:         runID,
		Action:        action,
		Status:        status,
		InputSummary:  input,
		OutputSummary: output,
	})
}

// recordDecision appends a step that applied the scout's eligibility
// rules, with the rules' verdict as its policy result and the reason as
// its output.
func (s *ScoutService) recordDecision(ctx context.Context, runID, action, policy, input, reason string) {
	s.save(ctx, &domain.AgentEvent{
		RunID:         runID,
		Action:        action,
		Status:        "ok",
		InputSummary:  input,
		OutputSummary: reason,
		PolicyResult:  policy,
	})
}

func (s *ScoutService) save(ctx context.Context, event *domain.AgentEvent) {
	event.Tool = "scout"
	event.CreatedAt = s.now()
	if err := s.events.Save(ctx, event); err != nil {
		slog.WarnContext(ctx, "failed to record scout event", "run_id", event.RunID, "action", event.Action, "error", err)
	}
}
