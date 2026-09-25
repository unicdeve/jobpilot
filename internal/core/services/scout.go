package services

import (
	"context"
	"fmt"
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
	Search(ctx context.Context, criteria domain.SearchCriteria) ([]*domain.Job, error)
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
	if err := scout.Validate(); err != nil {
		return err
	}
	if _, err := s.savedSearches.GetByID(ctx, scout.SavedSearchID); err != nil {
		return fmt.Errorf("the scout's saved search could not be found: %w", err)
	}
	return s.scouts.Save(ctx, scout)
}

// UpdateScout validates and persists changes to a Scout.
func (s *ScoutService) UpdateScout(ctx context.Context, scout *domain.Scout) error {
	if err := scout.Validate(); err != nil {
		return err
	}
	if _, err := s.savedSearches.GetByID(ctx, scout.SavedSearchID); err != nil {
		return fmt.Errorf("the scout's saved search could not be found: %w", err)
	}
	return s.scouts.Update(ctx, scout)
}

// ListScouts returns every Scout.
func (s *ScoutService) ListScouts(ctx context.Context) ([]*domain.Scout, error) {
	return s.scouts.List(ctx)
}

// GetScout returns one Scout.
func (s *ScoutService) GetScout(ctx context.Context, id string) (*domain.Scout, error) {
	return s.scouts.GetByID(ctx, id)
}

// DeleteScout removes a Scout.
func (s *ScoutService) DeleteScout(ctx context.Context, id string) error {
	return s.scouts.Delete(ctx, id)
}

// ListRuns returns recent scout runs, newest first.
func (s *ScoutService) ListRuns(ctx context.Context, limit int) ([]*domain.AgentRun, error) {
	return s.runs.ListUnattached(ctx, limit)
}

// ListRunEvents returns what one run did, in order.
func (s *ScoutService) ListRunEvents(ctx context.Context, runID string) ([]*domain.AgentEvent, error) {
	return s.events.ListByRun(ctx, runID)
}

// Run executes a Scout once.
//
// A run that fails partway keeps the applications it already created and
// records the failure (AC-006). Nothing here is transactional across jobs
// on purpose: an application already created is a real thing the user can
// act on, and undoing it because a later job failed to score would throw
// away good work for tidiness.
func (s *ScoutService) Run(ctx context.Context, scoutID string) (*domain.ScoutRunReport, error) {
	scout, err := s.scouts.GetByID(ctx, scoutID)
	if err != nil {
		return nil, fmt.Errorf("failed to load the scout: %w", err)
	}
	if !scout.IsEnabled {
		return nil, fmt.Errorf("scout %q is disabled", scout.Name)
	}

	search, err := s.savedSearches.GetByID(ctx, scout.SavedSearchID)
	if err != nil {
		return nil, fmt.Errorf("failed to load the scout's saved search: %w", err)
	}

	candidate, err := s.candidates.GetFirstCandidate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load the candidate profile: %w", err)
	}

	run := &domain.AgentRun{
		// No ApplicationID: a scout run creates several applications and
		// belongs to none of them (ADR-0018 §4).
		ScoutID:   scout.ID,
		TaskType:  ScoutTaskType,
		Status:    domain.AgentRunStatusRunning,
		StartedAt: s.now(),
	}
	if err := s.runs.Save(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to record the scout run: %w", err)
	}

	report := &domain.ScoutRunReport{ScoutID: scout.ID, RunID: run.ID}
	s.record(ctx, run.ID, "scout_started", "ok",
		fmt.Sprintf("scout %q, search %q", scout.Name, search.Name),
		fmt.Sprintf("floor %d, ceiling %d, source %s",
			scout.MinMatchScore, scout.MaxApplicationsPerRun, scout.Source))

	runErr := s.considerJobs(ctx, scout, search, candidate, run.ID, report)

	s.finish(ctx, run, report, runErr)
	if runErr != nil {
		report.Error = runErr.Error()
	}
	return report, runErr
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
		}

		if ok, reason := scout.PreScreen(input); !ok {
			verdict.Reason = reason
			report.Verdicts = append(report.Verdicts, verdict)
			s.recordDecision(ctx, runID, "job_rejected", domain.PolicyDenied, job.Title, reason)
			continue
		}

		match, err := s.matcher.AnalyzeJobForCandidate(ctx, candidate.ID, job.ID)
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
		s.record(ctx, runID, "sources_searched", "ok", search.Name,
			fmt.Sprintf("%d jobs returned", len(found)))
	}

	page, err := s.finder.ListJobs(ctx, domain.JobFilter{}, maxJobsConsideredPerRun, 0)
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
		Mode:        domain.ApplicationModeReview,
		State:       domain.ApplicationStateDiscovered,
	}
	if err := s.applications.Save(ctx, application); err != nil {
		return nil, err
	}
	return application, nil
}

// finish closes off the run record and the scout's last-run time.
func (s *ScoutService) finish(ctx context.Context, run *domain.AgentRun, report *domain.ScoutRunReport, runErr error) {
	completed := s.now()
	run.CompletedAt = &completed
	run.Status = domain.AgentRunStatusCompleted
	if runErr != nil {
		run.Status = domain.AgentRunStatusFailed
		run.Error = runErr.Error()
	}

	s.record(ctx, run.ID, "scout_finished", string(run.Status),
		fmt.Sprintf("%d jobs considered", report.Considered),
		fmt.Sprintf("%d applications created", report.Created))

	// A failure to write the trail must not discard the report: the
	// applications are already created and the user can see them.
	_ = s.runs.Update(ctx, run)

	if scout, err := s.scouts.GetByID(ctx, report.ScoutID); err == nil {
		scout.LastRunAt = &completed
		_ = s.scouts.Update(ctx, scout)
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
	_ = s.events.Save(ctx, event)
}
