package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

type fakeScoutRepository struct {
	scouts     map[string]*domain.Scout
	updateErr  error
	getByIDErr error
}

func newFakeScoutRepository() *fakeScoutRepository {
	return &fakeScoutRepository{scouts: map[string]*domain.Scout{}}
}

func (f *fakeScoutRepository) Save(_ context.Context, scout *domain.Scout) error {
	if scout.ID == "" {
		scout.ID = fmt.Sprintf("scout-%d", len(f.scouts)+1)
	}
	f.scouts[scout.ID] = scout
	return nil
}

func (f *fakeScoutRepository) GetByID(_ context.Context, id string) (*domain.Scout, error) {
	if f.getByIDErr != nil {
		return nil, f.getByIDErr
	}
	scout, ok := f.scouts[id]
	if !ok {
		return nil, fmt.Errorf("scout not found: %s", id)
	}
	return scout, nil
}

func (f *fakeScoutRepository) Update(_ context.Context, scout *domain.Scout) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.scouts[scout.ID] = scout
	return nil
}

func (f *fakeScoutRepository) List(context.Context) ([]*domain.Scout, error) {
	out := make([]*domain.Scout, 0, len(f.scouts))
	for _, scout := range f.scouts {
		out = append(out, scout)
	}
	return out, nil
}

func (f *fakeScoutRepository) Delete(_ context.Context, id string) error {
	delete(f.scouts, id)
	return nil
}

// fakeScoutJobFinder stands in for discovery. It records whether the
// sources were searched, which is how AC-003 is checked: a scout that
// stays local must not reach out at all.
type fakeScoutJobFinder struct {
	jobs         []*domain.Job
	sources      []domain.SourceOutcome
	searchCalls  int
	searchErr    error
	listErr      error
	searchedWith domain.SearchCriteria
}

func (f *fakeScoutJobFinder) Search(_ context.Context, criteria domain.SearchCriteria) (*domain.DiscoveryResult, error) {
	f.searchCalls++
	f.searchedWith = criteria
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return &domain.DiscoveryResult{Jobs: f.jobs, Sources: f.sources}, nil
}

func (f *fakeScoutJobFinder) ListJobs(_ context.Context, _ domain.JobFilter, _, _ int) (*domain.JobPage, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &domain.JobPage{Jobs: f.jobs, Total: len(f.jobs)}, nil
}

// fakeScoutMatcher scores jobs from a table.
type fakeScoutMatcher struct {
	scores map[string]int
	errs   map[string]error
	calls  []string
}

func (f *fakeScoutMatcher) AnalyzeJobForCandidate(_ context.Context, candidateID, jobID string) (*domain.JobMatch, error) {
	f.calls = append(f.calls, jobID)
	if err, ok := f.errs[jobID]; ok {
		return nil, err
	}
	score, ok := f.scores[jobID]
	if !ok {
		return nil, fmt.Errorf("no score for %s", jobID)
	}
	return &domain.JobMatch{JobID: jobID, CandidateID: candidateID, Score: score}, nil
}

// scoutFixture assembles a service with everything a run needs.
type scoutFixture struct {
	svc      *ScoutService
	scouts   *fakeScoutRepository
	apps     *fakeApplicationRepository
	finder   *fakeScoutJobFinder
	matcher  *fakeScoutMatcher
	runs     *fakeAgentRunRepository
	events   *fakeAgentEventRepository
	scout    *domain.Scout
	searchID string
}

func newScoutFixture(t *testing.T, jobs []*domain.Job, scores map[string]int,
	criteria domain.SearchCriteria, adjust func(*domain.Scout)) *scoutFixture {
	t.Helper()
	ctx := context.Background()

	searches := newFakeSavedSearchRepository()
	search := &domain.SavedSearch{ID: "search-1", Name: "Go roles", Criteria: criteria}
	require.NoError(t, searches.Save(ctx, search))

	candidates := newFakeCandidateRepository()
	candidate := &domain.Candidate{ID: "cand-1", FirstName: "Ada"}
	require.NoError(t, candidates.Save(ctx, candidate))

	scouts := newFakeScoutRepository()
	scout := &domain.Scout{
		Name: "Daily Go", SavedSearchID: search.ID, MinMatchScore: 70,
		MaxApplicationsPerRun: 10, Source: domain.ScoutSourceLocal, IsEnabled: true,
	}
	if adjust != nil {
		adjust(scout)
	}
	require.NoError(t, scouts.Save(ctx, scout))

	apps := newFakeApplicationRepository()
	finder := &fakeScoutJobFinder{jobs: jobs}
	matcher := &fakeScoutMatcher{scores: scores, errs: map[string]error{}}
	runs := newFakeAgentRunRepository()
	events := newFakeAgentEventRepository()

	return &scoutFixture{
		svc:    NewScoutService(scouts, searches, candidates, apps, runs, events, finder, matcher),
		scouts: scouts, apps: apps, finder: finder, matcher: matcher,
		runs: runs, events: events, scout: scout, searchID: search.ID,
	}
}

func job(id, title, company string) *domain.Job {
	return &domain.Job{ID: id, Title: title, CompanyName: company, Locations: []string{"Remote"}}
}

// TestScoutRun_OnlyCreatesForEligibleJobs is AC-001.
func TestScoutRun_OnlyCreatesForEligibleJobs(t *testing.T) {
	t.Parallel()

	jobs := []*domain.Job{
		job("job-1", "Senior Go Engineer", "Acme"),
		job("job-2", "Go Engineer", "Acme"),
		job("job-3", "Frontend Engineer", "Acme"),
	}
	scores := map[string]int{"job-1": 88, "job-2": 55, "job-3": 91}
	f := newScoutFixture(t, jobs, scores, domain.SearchCriteria{Keywords: "Go"}, nil)

	report, err := f.svc.Run(context.Background(), f.scout.ID)

	require.NoError(t, err)
	assert.Equal(t, 1, report.Created, "only the job that both matches the search and clears the floor")

	created, err := f.apps.ListByCandidate(context.Background(), "cand-1")
	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, "job-1", created[0].JobID)

	byJob := map[string]domain.ScoutVerdict{}
	for _, verdict := range report.Verdicts {
		byJob[verdict.JobID] = verdict
	}
	assert.Contains(t, byJob["job-2"].Reason, "below the scout's floor",
		"a job that matches but scores too low says so")
	assert.Contains(t, byJob["job-3"].Reason, "does not match the saved search",
		"a job the search excludes is rejected before it is scored")
	assert.NotContains(t, f.matcher.calls, "job-3",
		"scoring calls the model, so a job the search already excluded is not scored")
}

// TestScoutRun_CreatesNothingSubmittable is AC-002.
func TestScoutRun_CreatesNothingSubmittable(t *testing.T) {
	t.Parallel()

	f := newScoutFixture(t, []*domain.Job{job("job-1", "Go Engineer", "Acme")},
		map[string]int{"job-1": 95}, domain.SearchCriteria{Keywords: "Go"}, nil)

	_, err := f.svc.Run(context.Background(), f.scout.ID)
	require.NoError(t, err)

	created, err := f.apps.ListByCandidate(context.Background(), "cand-1")
	require.NoError(t, err)
	require.Len(t, created, 1)

	// Review is the only mode a scout produces, and the initial state is
	// the ordinary one a user-created application starts in. There is no
	// setting that changes either.
	assert.Equal(t, domain.ApplicationModeReview, created[0].Mode)
	assert.Equal(t, domain.ApplicationStateDiscovered, created[0].State)
	assert.True(t, created[0].SubmittedAt == nil || created[0].SubmittedAt.IsZero())
}

// TestScoutRun_ContactsNoEmployer is AC-003.
func TestScoutRun_ContactsNoEmployer(t *testing.T) {
	t.Parallel()

	t.Run("a local scout does not even reach the job sources", func(t *testing.T) {
		t.Parallel()
		f := newScoutFixture(t, []*domain.Job{job("job-1", "Go Engineer", "Acme")},
			map[string]int{"job-1": 95}, domain.SearchCriteria{Keywords: "Go"}, nil)

		_, err := f.svc.Run(context.Background(), f.scout.ID)

		require.NoError(t, err)
		assert.Zero(t, f.finder.searchCalls, "source 'local' considers what is already stored")
	})

	t.Run("a fetching scout searches the sources and nothing else", func(t *testing.T) {
		t.Parallel()
		f := newScoutFixture(t, []*domain.Job{job("job-1", "Go Engineer", "Acme")},
			map[string]int{"job-1": 95}, domain.SearchCriteria{Keywords: "Go"},
			func(s *domain.Scout) { s.Source = domain.ScoutSourceFetch })

		_, err := f.svc.Run(context.Background(), f.scout.ID)

		require.NoError(t, err)
		assert.Equal(t, 1, f.finder.searchCalls)
		assert.Equal(t, "Go", f.finder.searchedWith.Keywords)
	})

	t.Run("a fetching scout records each source that did not answer", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		f := newScoutFixture(t, []*domain.Job{job("job-1", "Go Engineer", "Acme")},
			map[string]int{"job-1": 95}, domain.SearchCriteria{Keywords: "Go"},
			func(s *domain.Scout) { s.Source = domain.ScoutSourceFetch })
		f.finder.sources = []domain.SourceOutcome{
			{SourceID: "remotive", SourceName: "Remotive", Status: domain.SourceStatusUnreachable},
			{SourceID: "remoteok", SourceName: "RemoteOK", Status: domain.SourceStatusFailed},
			{SourceID: "himalayas", SourceName: "Himalayas", Status: domain.SourceStatusOK, Found: 1},
		}

		report, err := f.svc.Run(ctx, f.scout.ID)
		require.NoError(t, err, "a source not answering does not fail the run")

		events, err := f.events.ListByRun(ctx, report.RunID)
		require.NoError(t, err)
		steps := map[string]*domain.AgentEvent{}
		for _, event := range events {
			steps[event.Action] = event
		}
		require.Contains(t, steps, "source_unreachable")
		assert.Equal(t, "Remotive", steps["source_unreachable"].InputSummary)
		assert.Equal(t, "error", steps["source_unreachable"].Status)
		require.Contains(t, steps, "source_failed")
		assert.Equal(t, "RemoteOK", steps["source_failed"].InputSummary)
		assert.Contains(t, steps["sources_searched"].OutputSummary, "1 of 3 sources answered")
	})

	t.Run("a fetching scout no source answered fails, saying why", func(t *testing.T) {
		t.Parallel()
		f := newScoutFixture(t, nil, nil, domain.SearchCriteria{Keywords: "Go"},
			func(s *domain.Scout) { s.Source = domain.ScoutSourceFetch })
		f.finder.searchErr = ErrSourcesUnreachable

		_, err := f.svc.Run(context.Background(), f.scout.ID)

		require.ErrorIs(t, err, ErrSourcesUnreachable)
		assert.Empty(t, f.matcher.calls, "nothing is scored when nothing was found")
	})

	// The stronger guarantee is structural rather than behavioural:
	// ScoutJobFinder is the only way a run can reach discovery, and it has
	// no method that opens a browser. ImportJob, which can, is absent by
	// construction. A test cannot assert the absence of a capability, so
	// this one names it where someone widening the interface will read it.
	t.Run("the finder interface offers no route to a browser", func(t *testing.T) {
		t.Parallel()
		var finder ScoutJobFinder = &fakeScoutJobFinder{}
		_, isImporter := finder.(interface {
			ImportJob(context.Context, string) (*domain.Job, error)
		})
		assert.False(t, isImporter, "a scout stops before the browser (ADR-0014)")
	})
}

// TestScoutRun_RecordsWhatItDid is AC-004.
func TestScoutRun_RecordsWhatItDid(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	jobs := []*domain.Job{
		job("job-1", "Senior Go Engineer", "Acme"),
		job("job-2", "Go Engineer", "Acme"),
	}
	f := newScoutFixture(t, jobs, map[string]int{"job-1": 88, "job-2": 40},
		domain.SearchCriteria{Keywords: "Go"}, nil)

	report, err := f.svc.Run(ctx, f.scout.ID)
	require.NoError(t, err)
	require.NotEmpty(t, report.RunID)

	run, err := f.runs.GetByID(ctx, report.RunID)
	require.NoError(t, err)
	assert.Equal(t, ScoutTaskType, run.TaskType)
	assert.Equal(t, domain.AgentRunStatusCompleted, run.Status)
	assert.Empty(t, run.ApplicationID,
		"a scout run creates several applications and belongs to none of them")
	assert.Equal(t, f.scout.ID, run.ScoutID, "so it names the scout instead")
	require.NotNil(t, run.CompletedAt)

	events, err := f.events.ListByRun(ctx, report.RunID)
	require.NoError(t, err)

	actions := map[string]string{}
	policies := map[string]string{}
	for _, event := range events {
		actions[event.Action] = event.OutputSummary
		policies[event.Action] = event.PolicyResult
	}
	assert.Equal(t, domain.PolicyDenied, policies["job_rejected"],
		"an eligibility decision is recorded as the step's policy result")
	assert.Equal(t, domain.PolicyAllowed, policies["application_created"])
	assert.Empty(t, policies["jobs_collected"], "a step no rule decided has none")
	assert.Contains(t, actions, "scout_started")
	assert.Contains(t, actions, "jobs_collected")
	assert.Contains(t, actions, "application_created")
	require.Contains(t, actions, "job_rejected")
	assert.Contains(t, actions["job_rejected"], "below the scout's floor",
		"the trail records why, not just that")
	assert.Contains(t, actions, "scout_finished")

	// The scout's own last-run time is updated so the user can see it ran.
	scout, err := f.scouts.GetByID(ctx, f.scout.ID)
	require.NoError(t, err)
	require.NotNil(t, scout.LastRunAt)
}

// TestScoutRun_HonoursItsCeiling is AC-005.
func TestScoutRun_HonoursItsCeiling(t *testing.T) {
	t.Parallel()

	jobs := []*domain.Job{
		job("job-1", "Go Engineer A", "Acme"),
		job("job-2", "Go Engineer B", "Acme"),
		job("job-3", "Go Engineer C", "Acme"),
		job("job-4", "Go Engineer D", "Acme"),
	}
	scores := map[string]int{"job-1": 90, "job-2": 91, "job-3": 92, "job-4": 93}
	f := newScoutFixture(t, jobs, scores, domain.SearchCriteria{Keywords: "Go"},
		func(s *domain.Scout) { s.MaxApplicationsPerRun = 2 })

	report, err := f.svc.Run(context.Background(), f.scout.ID)

	require.NoError(t, err)
	assert.Equal(t, 2, report.Created)
	assert.True(t, report.CeilingHit)

	created, err := f.apps.ListByCandidate(context.Background(), "cand-1")
	require.NoError(t, err)
	assert.Len(t, created, 2)

	assert.Len(t, f.matcher.calls, 2,
		"a run that has made its quota stops looking rather than scoring jobs it cannot act on")
}

// TestScoutRun_PartialFailureKeepsItsWork is AC-006.
func TestScoutRun_PartialFailureKeepsItsWork(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a job that cannot be scored does not end the run", func(t *testing.T) {
		t.Parallel()
		jobs := []*domain.Job{
			job("job-1", "Go Engineer A", "Acme"),
			job("job-2", "Go Engineer B", "Acme"),
			job("job-3", "Go Engineer C", "Acme"),
		}
		f := newScoutFixture(t, jobs, map[string]int{"job-1": 90, "job-3": 92},
			domain.SearchCriteria{Keywords: "Go"}, nil)
		f.matcher.errs["job-2"] = errors.New("the model is unreachable")

		report, err := f.svc.Run(ctx, f.scout.ID)

		require.NoError(t, err)
		assert.Equal(t, 2, report.Created, "the jobs either side of the failure are still considered")

		events, err := f.events.ListByRun(ctx, report.RunID)
		require.NoError(t, err)
		var recorded bool
		for _, event := range events {
			if event.Action == "job_scoring_failed" {
				recorded = true
				assert.Equal(t, "error", event.Status)
			}
		}
		assert.True(t, recorded, "the failure is recorded rather than swallowed")
	})

	t.Run("a failure partway keeps the applications already created", func(t *testing.T) {
		t.Parallel()
		jobs := []*domain.Job{
			job("job-1", "Go Engineer A", "Acme"),
			job("job-2", "Go Engineer B", "Acme"),
		}
		f := newScoutFixture(t, jobs, map[string]int{"job-1": 90, "job-2": 91},
			domain.SearchCriteria{Keywords: "Go"}, nil)
		f.apps.saveErrAfter = 1

		report, err := f.svc.Run(ctx, f.scout.ID)

		require.Error(t, err)
		require.NotNil(t, report, "the report survives the failure so the user can see what was done")
		assert.Equal(t, 1, report.Created)
		assert.Contains(t, report.Error, "failed to create an application")

		created, listErr := f.apps.ListByCandidate(ctx, "cand-1")
		require.NoError(t, listErr)
		assert.Len(t, created, 1, "work already done is not undone for tidiness")

		run, runErr := f.runs.GetByID(ctx, report.RunID)
		require.NoError(t, runErr)
		assert.Equal(t, domain.AgentRunStatusFailed, run.Status)
		assert.NotEmpty(t, run.Error)
	})
}

func TestScoutRun_SkipsJobsAlreadyApplied(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	f := newScoutFixture(t, []*domain.Job{job("job-1", "Go Engineer", "Acme")},
		map[string]int{"job-1": 95}, domain.SearchCriteria{Keywords: "Go"}, nil)
	require.NoError(t, f.apps.Save(ctx, &domain.Application{JobID: "job-1", CandidateID: "cand-1"}))

	report, err := f.svc.Run(ctx, f.scout.ID)

	require.NoError(t, err)
	assert.Zero(t, report.Created)
	require.Len(t, report.Verdicts, 1)
	assert.Contains(t, report.Verdicts[0].Reason, "already exists")
	assert.Empty(t, f.matcher.calls, "a job already applied to is not scored again")
}

func TestScoutRun_RefusesWhenDisabled(t *testing.T) {
	t.Parallel()

	f := newScoutFixture(t, nil, nil, domain.SearchCriteria{Keywords: "Go"},
		func(s *domain.Scout) { s.IsEnabled = false })

	_, err := f.svc.Run(context.Background(), f.scout.ID)

	assert.ErrorContains(t, err, "disabled")
}

func TestScoutService_CreateScout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("fills in the defaults a caller may omit", func(t *testing.T) {
		t.Parallel()
		f := newScoutFixture(t, nil, nil, domain.SearchCriteria{Keywords: "Go"}, nil)

		scout := &domain.Scout{Name: "New", SavedSearchID: f.searchID}
		require.NoError(t, f.svc.CreateScout(ctx, scout))

		assert.Equal(t, domain.ScoutSourceLocal, scout.Source)
		assert.Equal(t, 10, scout.MaxApplicationsPerRun)
	})

	t.Run("refuses a scout whose saved search does not exist", func(t *testing.T) {
		t.Parallel()
		f := newScoutFixture(t, nil, nil, domain.SearchCriteria{Keywords: "Go"}, nil)

		err := f.svc.CreateScout(ctx, &domain.Scout{Name: "New", SavedSearchID: "nope"})

		assert.ErrorContains(t, err, "saved search could not be found")
	})

	t.Run("refuses an invalid scout", func(t *testing.T) {
		t.Parallel()
		f := newScoutFixture(t, nil, nil, domain.SearchCriteria{Keywords: "Go"}, nil)

		err := f.svc.CreateScout(ctx, &domain.Scout{Name: "  ", SavedSearchID: f.searchID})

		assert.ErrorContains(t, err, "needs a name")
	})
}
