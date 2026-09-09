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

	t.Run("zero extracted fields is treated as a blocker", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		worker.fields = nil

		err := svc.PrepareApplication(context.Background(), appID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "no recognizable application form fields")
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
		worker.fields = []ports.FormField{{Ref: "0", Label: "Full name", Kind: ports.FormFieldKindText}}

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

		worker.fields = []ports.FormField{{Ref: "0", Label: "Full name", Kind: ports.FormFieldKindText}}

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
