package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func newTestBrowserWorkerService(t *testing.T) (
	svc *BrowserWorkerService,
	appRepo *fakeApplicationRepository,
	jobRepo *fakeJobRepository,
	candRepo *fakeCandidateRepository,
	runRepo *fakeAgentRunRepository,
	worker *fakeBrowserWorker,
) {
	t.Helper()
	appRepo = newFakeApplicationRepository()
	jobRepo = newFakeJobRepository()
	candRepo = newFakeCandidateRepository()
	questionRepo := newFakeApplicationQuestionRepository()
	runRepo = newFakeAgentRunRepository()
	eventRepo := newFakeAgentEventRepository()
	worker = &fakeBrowserWorker{}

	appSvc := NewApplicationService(appRepo)
	answers := NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, newFakeResumeRepository(), &fakeLLMProvider{providerName: "fake-llm"})

	svc = NewBrowserWorkerService(appSvc, jobRepo, runRepo, eventRepo, answers, func() (ports.BrowserWorker, error) { return worker, nil })
	return svc, appRepo, jobRepo, candRepo, runRepo, worker
}

// seedPreparingApplication saves candidate and job, then an application in
// PREPARING state referencing them, returning the application's ID.
func seedPreparingApplication(
	t *testing.T,
	appRepo *fakeApplicationRepository,
	jobRepo *fakeJobRepository,
	candRepo *fakeCandidateRepository,
	candidate *domain.Candidate,
	job *domain.Job,
) string {
	t.Helper()
	require.NoError(t, candRepo.Save(context.Background(), candidate))
	require.NoError(t, jobRepo.Save(context.Background(), job))

	app := &domain.Application{
		JobID:       job.ID,
		CandidateID: candidate.ID,
		State:       domain.ApplicationStatePreparing,
	}
	require.NoError(t, appRepo.Save(context.Background(), app))
	return app.ID
}

// applicationFormFields is a page that asks about a person, which is what
// the gate in reachApplicationForm requires before anything is filled.
func applicationFormFields() []ports.FormField {
	return []ports.FormField{
		{Ref: "0", Label: "Full name", Kind: ports.FormFieldKindText},
		{Ref: "1", Label: "Email", Kind: ports.FormFieldKindText},
		{Ref: "2", Label: "Phone", Kind: ports.FormFieldKindText},
	}
}

func TestBrowserWorkerService_PrepareApplication(t *testing.T) {
	t.Parallel()

	t.Run("extracts, resolves, and fills known fields, then transitions to READY_FOR_REVIEW", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{FirstName: "Ada", LastName: "Lovelace"}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		worker.fields = []ports.FormField{
			{Ref: "0", Label: "Full name", Kind: ports.FormFieldKindText, Required: true},
			{Ref: "1", Label: "Why do you want to work here?", Kind: ports.FormFieldKindTextarea, Required: false},
			{Ref: "2", Label: "Email", Kind: ports.FormFieldKindText, Required: true},
			{Ref: "3", Label: "Phone", Kind: ports.FormFieldKindText},
		}

		err := svc.PrepareApplication(context.Background(), appID)
		require.NoError(t, err)

		assert.Equal(t, domain.ApplicationStateReadyForReview, appRepo.applications[appID].State)
		assert.Equal(t, "Ada Lovelace", worker.filled["0"])
		_, textareaWasFilled := worker.filled["1"]
		assert.False(t, textareaWasFilled, "the narrative question has no LLM-backed answer in this fake, so it should be left for the user")
		assert.False(t, worker.closed, "the browser must stay open for the user to review")

		runs, err := svc.ListRuns(context.Background(), appID)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, domain.AgentRunStatusCompleted, runs[0].Status)

		events, err := svc.ListEvents(context.Background(), runs[0].ID)
		require.NoError(t, err)
		assert.NotEmpty(t, events)

		_ = runRepo // exercised via svc.ListRuns above
	})

	t.Run("a detected blocker stops the run and fails the application without closing the browser", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		worker.blocked = true
		worker.blockedReason = "a recaptcha challenge is present on the page"

		err := svc.PrepareApplication(context.Background(), appID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "recaptcha")

		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
		assert.False(t, worker.closed)

		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, domain.AgentRunStatusBlocked, runs[0].Status)
		assert.Contains(t, runs[0].Error, "recaptcha")
	})

	t.Run("a page that is not an application form is not filled in", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		worker.fields = nil

		err := svc.PrepareApplication(context.Background(), appID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "could not find a way to the application form")
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)

		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, domain.AgentRunStatusBlocked, runs[0].Status)
	})

	t.Run("a navigation failure stops the run without extracting or filling anything", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		worker.navigateErr = assert.AnError
		worker.fields = applicationFormFields()

		err := svc.PrepareApplication(context.Background(), appID)
		require.Error(t, err)
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
		assert.Empty(t, worker.filled)
	})

	t.Run("application not currently in PREPARING is rejected by the existing state machine, not duplicated here", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)
		appRepo.applications[appID].State = domain.ApplicationStateDiscovered

		worker.fields = applicationFormFields()

		err := svc.PrepareApplication(context.Background(), appID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "cannot transition application")
	})

	t.Run("a browser that fails to launch is recorded as a failed run, distinct from a content blocker", func(t *testing.T) {
		t.Parallel()
		appRepo := newFakeApplicationRepository()
		jobRepo := newFakeJobRepository()
		candRepo := newFakeCandidateRepository()
		questionRepo := newFakeApplicationQuestionRepository()
		runRepo := newFakeAgentRunRepository()
		eventRepo := newFakeAgentEventRepository()

		appSvc := NewApplicationService(appRepo)
		answers := NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, newFakeResumeRepository(), &fakeLLMProvider{providerName: "fake-llm"})
		svc := NewBrowserWorkerService(appSvc, jobRepo, runRepo, eventRepo, answers, func() (ports.BrowserWorker, error) {
			return nil, assert.AnError
		})

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		err := svc.PrepareApplication(context.Background(), appID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to launch the browser")

		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)

		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, domain.AgentRunStatusFailed, runs[0].Status)
	})
}

// TestBrowserWorkerService_ReachesTheApplicationForm covers the route from
// the job posting to the form, which is what "Prepare in browser" means.
func TestBrowserWorkerService_ReachesTheApplicationForm(t *testing.T) {
	t.Parallel()

	newsletterOnly := []ports.FormField{
		{Ref: "0", Label: "Enter your email", Kind: ports.FormFieldKindText},
	}

	t.Run("follows the apply control from a posting to the employer's form", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)
		candidate := &domain.Candidate{FirstName: "Ada", LastName: "Lovelace"}
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		// The shape of the live route: a listing page carrying only the
		// board's newsletter box, then the real form one hop away.
		worker.pages = [][]ports.FormField{newsletterOnly, applicationFormFields()}
		worker.controls = []ports.ApplyControl{
			{Ref: "0", Text: "Jobs in Berlin", URL: "https://board.example/berlin"},
			{Ref: "1", Text: "Apply Now", URL: "https://jobs.ashbyhq.com/acme/1/application"},
		}

		require.NoError(t, svc.PrepareApplication(context.Background(), appID))

		assert.Equal(t, []string{"1"}, worker.followed, "it must follow the apply control, not the city link")
		assert.Equal(t, domain.ApplicationStateReadyForReview, appRepo.applications[appID].State)
		assert.Equal(t, "Ada Lovelace", worker.filled["0"], "the form it reached is the one it fills")
	})

	t.Run("refuses to fill a newsletter box and call it an application", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)
		candidate := &domain.Candidate{FirstName: "Ada", Contact: "ada@example.com"}
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		// Every hop is the same listing page: nothing here is a form.
		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = []ports.ApplyControl{
			{Ref: "0", Text: "Apply Now", URL: "https://board.example/jobs/1/apply"},
		}

		err := svc.PrepareApplication(context.Background(), appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "could not find the application form")
		assert.Empty(t, worker.filled, "the candidate's details must never reach a page that is not an application form")
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
	})

	t.Run("stops after the hop limit rather than wandering", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{}, job)

		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = []ports.ApplyControl{
			{Ref: "0", Text: "Apply", URL: "https://board.example/loop"},
		}

		require.Error(t, svc.PrepareApplication(context.Background(), appID))

		assert.Len(t, worker.followed, maxApplyHops, "a redirect loop must not be followed forever")
	})

	t.Run("says so when no control on the page looks like a way in", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{}, job)

		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = []ports.ApplyControl{{Ref: "0", Text: "Privacy policy", URL: "https://board.example/privacy"}}

		err := svc.PrepareApplication(context.Background(), appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "no link or button on it looks like an apply control")
		assert.Empty(t, worker.followed)
	})

	t.Run("a posting that is itself the form needs no hop", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)
		job := &domain.Job{CanonicalURL: "https://acme.example/careers/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{FirstName: "Ada"}, job)

		worker.pages = [][]ports.FormField{applicationFormFields()}

		require.NoError(t, svc.PrepareApplication(context.Background(), appID))
		assert.Empty(t, worker.followed, "a company career page often holds the form itself")
	})
}
