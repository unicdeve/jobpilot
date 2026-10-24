package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

	appSvc := NewApplicationService(appRepo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
	answers := NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, newFakeResumeRepository(), &fakeLLMProvider{providerName: "fake-llm"}, nil)

	svc = NewBrowserWorkerService(appSvc, jobRepo, runRepo, eventRepo, answers,
		func() (ports.BrowserWorker, error) { return worker, nil }, nil)
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

// prepareAndWait starts a run, waits for it to finish, and returns how it
// ended as an error: nil for a completed run, its recorded reason for any
// other, or the refusal to start.
func prepareAndWait(svc *BrowserWorkerService, applicationID string) error {
	run, err := svc.StartPreparation(context.Background(), applicationID)
	if err != nil {
		return err
	}
	svc.Wait()
	finished, err := svc.runRepo.GetByID(context.Background(), run.ID)
	if err != nil {
		return err
	}
	if finished.Status != domain.AgentRunStatusCompleted {
		return errors.New(finished.Error)
	}
	return nil
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

		err := prepareAndWait(svc, appID)
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

	t.Run("an Assist application is refused before a browser opens", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)

		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{}, &domain.Job{CanonicalURL: "https://acme.example/jobs/1"})
		appRepo.applications[appID].Mode = domain.ApplicationModeAssist
		worker.fields = applicationFormFields()

		err := prepareAndWait(svc, appID)
		require.ErrorIs(t, err, ErrAssistDoesNotFillForms)

		assert.Empty(t, worker.navigated, "no page was opened")
		assert.Empty(t, worker.filled)
		assert.Equal(t, domain.ApplicationStatePreparing, appRepo.applications[appID].State, "nothing was attempted, so nothing failed")
		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		assert.Empty(t, runs)
	})

	t.Run("a page that is not an application form is not filled in", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		worker.fields = nil

		err := prepareAndWait(svc, appID)
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

		err := prepareAndWait(svc, appID)
		require.Error(t, err)
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
		assert.Empty(t, worker.filled)
	})

	t.Run("an application that cannot be prepared is refused before a run starts", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)
		appRepo.applications[appID].State = domain.ApplicationStateDiscovered

		worker.fields = applicationFormFields()

		err := prepareAndWait(svc, appID)
		require.ErrorIs(t, err, ErrCannotPrepare)
		assert.ErrorContains(t, err, "this one is discovered")

		assert.Empty(t, worker.navigated, "no page was opened")
		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		assert.Empty(t, runs)
	})

	t.Run("a failed application is moved back to preparing and tried again", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)

		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{FirstName: "Ada"}, job)
		appRepo.applications[appID].State = domain.ApplicationStateFailed
		worker.fields = applicationFormFields()

		require.NoError(t, prepareAndWait(svc, appID))
		assert.Equal(t, domain.ApplicationStateReadyForReview, appRepo.applications[appID].State)
	})

	t.Run("starts from the link the user gave rather than the job's own", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)

		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{FirstName: "Ada"}, job)
		appRepo.applications[appID].ApplyURL = "https://jobs.ashbyhq.com/acme/1/application"
		worker.fields = applicationFormFields()

		require.NoError(t, prepareAndWait(svc, appID))
		assert.Equal(t, []string{"https://jobs.ashbyhq.com/acme/1/application"}, worker.navigated)
	})

	t.Run("a browser that fails to launch is recorded as a failed run, distinct from a content blocker", func(t *testing.T) {
		t.Parallel()
		appRepo := newFakeApplicationRepository()
		jobRepo := newFakeJobRepository()
		candRepo := newFakeCandidateRepository()
		questionRepo := newFakeApplicationQuestionRepository()
		runRepo := newFakeAgentRunRepository()
		eventRepo := newFakeAgentEventRepository()

		appSvc := NewApplicationService(appRepo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
		answers := NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, newFakeResumeRepository(), &fakeLLMProvider{providerName: "fake-llm"}, nil)
		svc := NewBrowserWorkerService(appSvc, jobRepo, runRepo, eventRepo, answers, func() (ports.BrowserWorker, error) {
			return nil, assert.AnError
		}, nil)

		candidate := &domain.Candidate{}
		job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		err := prepareAndWait(svc, appID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to launch the browser")

		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)

		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, domain.AgentRunStatusFailed, runs[0].Status)
	})
}

// TestBrowserWorkerService_TrailHoldsNoAnswers covers the rule in
// docs/02_features/12_agent_activity.md that the activity trail shows no
// unnecessary candidate data. The answers are stored on the application's
// questions; the trail says a field was filled, not what went into it.
func TestBrowserWorkerService_TrailHoldsNoAnswers(t *testing.T) {
	t.Parallel()
	svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)

	candidate := &domain.Candidate{
		FirstName: "Ada", LastName: "Lovelace",
		Email: "ada@example.com", Phone: "+44 20 7946 0000",
	}
	job := &domain.Job{CanonicalURL: "https://acme.example/jobs/1"}
	appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)
	worker.fields = applicationFormFields()

	require.NoError(t, prepareAndWait(svc, appID))
	require.Equal(t, "ada@example.com", worker.filled["1"], "the answer did reach the form")

	runs, err := svc.ListRuns(context.Background(), appID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	events, err := svc.ListEvents(context.Background(), runs[0].ID)
	require.NoError(t, err)

	filled := 0
	for _, event := range events {
		for _, text := range []string{event.InputSummary, event.OutputSummary} {
			for _, answer := range []string{"Ada Lovelace", "ada@example.com", "+44 20 7946 0000"} {
				assert.NotContains(t, text, answer, "step %s recorded an answer", event.Action)
			}
		}
		if event.Action == "fill_field" {
			filled++
			assert.Empty(t, event.OutputSummary)
		}
		if event.Action == "extract_form" {
			assert.Equal(t, domain.PolicyAllowed, event.PolicyResult,
				"the application-form gate's verdict is the step's policy result")
		}
	}
	assert.Equal(t, 3, filled)
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

		require.NoError(t, prepareAndWait(svc, appID))

		assert.Equal(t, []string{"1"}, worker.followed, "it must follow the apply control, not the city link")
		assert.Equal(t, domain.ApplicationStateReadyForReview, appRepo.applications[appID].State)
		assert.Equal(t, "Ada Lovelace", worker.filled["0"], "the form it reached is the one it fills")
	})

	t.Run("refuses to fill a newsletter box and call it an application", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)
		candidate := &domain.Candidate{FirstName: "Ada", Email: "ada@example.com"}
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, candidate, job)

		// Every hop is the same listing page: nothing here is a form.
		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = []ports.ApplyControl{
			{Ref: "0", Text: "Apply Now", URL: "https://board.example/jobs/1/apply"},
		}

		err := prepareAndWait(svc, appID)

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

		require.Error(t, prepareAndWait(svc, appID))

		assert.Len(t, worker.followed, maxApplyHops, "a redirect loop must not be followed forever")
	})

	t.Run("says so when no control on the page looks like a way in", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, jobRepo, candRepo, _, worker := newTestBrowserWorkerService(t)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{}, job)

		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = []ports.ApplyControl{{Ref: "0", Text: "Privacy policy", URL: "https://board.example/privacy"}}

		err := prepareAndWait(svc, appID)

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

		require.NoError(t, prepareAndWait(svc, appID))
		assert.Empty(t, worker.followed, "a company career page often holds the form itself")
	})
}

// newBrowserWorkerServiceWithChooser builds the service with the LLM
// fallback wired in, so the tests below can watch when the model is asked
// and when it is not.
func newBrowserWorkerServiceWithChooser(t *testing.T, llm *fakeLLMProvider) (
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
	runRepo = newFakeAgentRunRepository()
	eventRepo := newFakeAgentEventRepository()
	worker = &fakeBrowserWorker{}

	appSvc := NewApplicationService(appRepo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
	answers := NewAnswerService(newFakeApplicationQuestionRepository(), appRepo, jobRepo, candRepo,
		newFakeResumeRepository(), &fakeLLMProvider{providerName: "fake-llm"}, nil)

	svc = NewBrowserWorkerService(appSvc, jobRepo, runRepo, eventRepo, answers,
		func() (ports.BrowserWorker, error) { return worker, nil }, NewApplyControlChooser(llm))
	return svc, appRepo, jobRepo, candRepo, runRepo, worker
}

// TestBrowserWorkerService_ApplyControlFallback covers the half of ADR-0013
// that involves a model: it is asked only when the deterministic rules find
// nothing, it can only pick from what the page offered, and whatever it
// says the form gate still decides whether anything gets filled.
func TestBrowserWorkerService_ApplyControlFallback(t *testing.T) {
	t.Parallel()

	newsletterOnly := []ports.FormField{
		{Ref: "0", Label: "Enter your email", Kind: ports.FormFieldKindText},
	}

	// A way in worded like nothing in the apply vocabulary, pointing at no
	// host we recognise — which is the only situation the model is for.
	unreadableWayIn := []ports.ApplyControl{
		{Ref: "0", Text: "Home", URL: "https://board.example/"},
		{Ref: "1", Text: "Bli med i teamet", URL: "https://board.example/rekruttering/4821"},
	}

	t.Run("the model is not asked when the rules already found the way in", func(t *testing.T) {
		t.Parallel()
		llm := &fakeLLMProvider{response: ports.Response{Content: `{"index": 0}`}}
		svc, appRepo, jobRepo, candRepo, _, worker := newBrowserWorkerServiceWithChooser(t, llm)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{FirstName: "Ada"}, job)

		worker.pages = [][]ports.FormField{newsletterOnly, applicationFormFields()}
		worker.controls = []ports.ApplyControl{
			{Ref: "0", Text: "Jobs in Berlin", URL: "https://board.example/berlin"},
			{Ref: "1", Text: "Apply Now", URL: "https://jobs.ashbyhq.com/acme/1/application"},
		}

		require.NoError(t, prepareAndWait(svc, appID))

		assert.Equal(t, []string{"1"}, worker.followed)
		assert.Zero(t, llm.generateCalls, "the common case must stay deterministic and offline")
	})

	t.Run("the model is asked when they do not, and the run follows its choice", func(t *testing.T) {
		t.Parallel()
		llm := &fakeLLMProvider{response: ports.Response{Content: `{"index": 1}`}}
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newBrowserWorkerServiceWithChooser(t, llm)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{FirstName: "Ada"}, job)

		worker.pages = [][]ports.FormField{newsletterOnly, applicationFormFields()}
		worker.controls = unreadableWayIn

		require.NoError(t, prepareAndWait(svc, appID))

		assert.Equal(t, 1, llm.generateCalls)
		assert.Equal(t, []string{"1"}, worker.followed)
		assert.Equal(t, domain.ApplicationStateReadyForReview, appRepo.applications[appID].State)

		// ADR-0013 §2: the trail has to say a model was involved.
		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		events, err := svc.ListEvents(context.Background(), runs[0].ID)
		require.NoError(t, err)
		assert.True(t, hasEvent(events, "choose_apply", "llm"), "the model's involvement must be on the record")
		assert.True(t, hasEventOutputContaining(events, "follow_apply", "chosen by the model"))
	})

	t.Run("the model abstaining stops the run instead of guessing", func(t *testing.T) {
		t.Parallel()
		llm := &fakeLLMProvider{response: ports.Response{Content: `{"index": -1}`}}
		svc, appRepo, jobRepo, candRepo, _, worker := newBrowserWorkerServiceWithChooser(t, llm)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{Email: "ada@example.com"}, job)

		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = unreadableWayIn

		err := prepareAndWait(svc, appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "no link or button on it looks like an apply control")
		assert.Empty(t, worker.followed)
		assert.Empty(t, worker.filled, "the candidate's details must not reach the newsletter box")
	})

	t.Run("a model that cannot be reached leaves the run where it would have been anyway", func(t *testing.T) {
		t.Parallel()
		llm := &fakeLLMProvider{generateErr: assert.AnError}
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newBrowserWorkerServiceWithChooser(t, llm)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{}, job)

		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = unreadableWayIn

		err := prepareAndWait(svc, appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "no link or button on it looks like an apply control",
			"an unconfigured or unreachable model is not what the user needs to be told about here")
		assert.Empty(t, worker.followed)

		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		events, err := svc.ListEvents(context.Background(), runs[0].ID)
		require.NoError(t, err)
		assert.True(t, hasEvent(events, "choose_apply", "llm"), "the attempt still belongs on the record")
	})

	t.Run("the page the model lands on still has to be a form", func(t *testing.T) {
		t.Parallel()
		llm := &fakeLLMProvider{response: ports.Response{Content: `{"index": 0}`}}
		svc, appRepo, jobRepo, candRepo, _, worker := newBrowserWorkerServiceWithChooser(t, llm)
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{Email: "ada@example.com"}, job)

		// Wherever the model sends it, every page is the same newsletter box.
		worker.pages = [][]ports.FormField{newsletterOnly}
		worker.controls = unreadableWayIn

		err := prepareAndWait(svc, appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "could not find the application form")
		assert.Empty(t, worker.filled, "the gate protects the model's choices exactly as it protects the rules'")
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
	})
}

func hasEvent(events []*domain.AgentEvent, action, tool string) bool {
	for _, event := range events {
		if event.Action == action && event.Tool == tool {
			return true
		}
	}
	return false
}

func hasEventOutputContaining(events []*domain.AgentEvent, action, want string) bool {
	for _, event := range events {
		if event.Action == action && strings.Contains(event.OutputSummary, want) {
			return true
		}
	}
	return false
}

// TestBrowserWorkerService_WaitsForTheUser covers ADR-0033: a page asking
// whether the user is a person pauses the run until they answer it, and a
// site that asks again after being answered is refusing JobPilot's browser.
func TestBrowserWorkerService_WaitsForTheUser(t *testing.T) {
	t.Parallel()

	const checking = "board.example is checking that you're a person"

	// newWaitingService is the service with waits short enough for a test,
	// and a job whose posting is the application form once the check is out
	// of the way.
	newWaitingService := func(t *testing.T, timeout time.Duration) (
		*BrowserWorkerService, *fakeApplicationRepository, *fakeAgentRunRepository, *fakeBrowserWorker, string,
	) {
		t.Helper()
		svc, appRepo, jobRepo, candRepo, runRepo, worker := newTestBrowserWorkerService(t)
		svc.challengePoll = time.Millisecond
		svc.challengeTimeout = timeout
		job := &domain.Job{CanonicalURL: "https://board.example/jobs/1"}
		appID := seedPreparingApplication(t, appRepo, jobRepo, candRepo, &domain.Candidate{FirstName: "Ada"}, job)
		worker.fields = applicationFormFields()
		worker.currentURL = job.CanonicalURL
		return svc, appRepo, runRepo, worker, appID
	}

	// alwaysChecking is a check nobody answers. asked is closed the first
	// time the run looks while waiting, so a test can act on a waiting run
	// without reading what the run goroutine is writing.
	alwaysChecking := func(asked chan<- struct{}) func(int) (string, bool) {
		return func(call int) (string, bool) {
			if call == 1 {
				close(asked)
			}
			return checking, true
		}
	}

	t.Run("a check the user answers pauses the run, which then carries on", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, runRepo, worker, appID := newWaitingService(t, time.Minute)
		// Checking for the first look and three while waiting, then gone.
		worker.detect = func(call int) (string, bool) { return checking, call < 4 }

		require.NoError(t, prepareAndWait(svc, appID))

		assert.True(t, worker.focused, "the window the user has to answer in is brought forward")
		assert.Equal(t, "Ada", worker.filled["0"])
		assert.Equal(t, domain.ApplicationStateReadyForReview, appRepo.applications[appID].State)

		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, domain.AgentRunStatusCompleted, runs[0].Status)
		events, err := svc.ListEvents(context.Background(), runs[0].ID)
		require.NoError(t, err)
		assert.True(t, hasEventOutputContaining(events, "detect_blocker", "waiting for you"))
		assert.True(t, hasEvent(events, "solve_check", "user"), "the trail says the user answered it, not JobPilot")
	})

	t.Run("one clear look is not an answer: the page may be between documents", func(t *testing.T) {
		t.Parallel()
		svc, _, _, worker, appID := newWaitingService(t, 50*time.Millisecond)
		// Every other look comes back clear, as a page mid-reload can.
		worker.detect = func(call int) (string, bool) { return checking, call%2 == 0 }

		err := prepareAndWait(svc, appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "was not answered within")
		assert.Empty(t, worker.filled)
	})

	t.Run("a check still there after the page reloads is the site refusing the browser", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, runRepo, worker, appID := newWaitingService(t, time.Minute)
		worker.detect = func(call int) (string, bool) {
			if call == 3 {
				worker.loads++ // the user's answer was sent, and the check came back
			}
			return checking, true
		}

		err := prepareAndWait(svc, appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "board.example asked again after its check was answered")
		assert.Empty(t, worker.filled)
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
		runs, err := runRepo.ListByApplication(context.Background(), appID)
		require.NoError(t, err)
		assert.Equal(t, domain.AgentRunStatusBlocked, runs[0].Status)
	})

	t.Run("a check that clears and comes back on the same page is the site refusing the browser", func(t *testing.T) {
		t.Parallel()
		svc, _, _, worker, appID := newWaitingService(t, time.Minute)
		// Checking, then two clear looks, then checking again when the run
		// looks at the page before reading it.
		worker.detect = func(call int) (string, bool) { return checking, call == 0 || call >= 3 }

		err := prepareAndWait(svc, appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "asked again after its check was answered")
		assert.Empty(t, worker.filled)
	})

	t.Run("a check nobody answers ends the run when the wait runs out", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, _, worker, appID := newWaitingService(t, 20*time.Millisecond)
		worker.blocked, worker.blockedReason = true, checking

		err := prepareAndWait(svc, appID)

		require.Error(t, err)
		assert.ErrorContains(t, err, checking+", and it was not answered within")
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
	})

	t.Run("the user can stop a waiting run, and only once", func(t *testing.T) {
		t.Parallel()
		svc, appRepo, runRepo, worker, appID := newWaitingService(t, time.Minute)
		asked := make(chan struct{})
		worker.detect = alwaysChecking(asked)

		run, err := svc.StartPreparation(context.Background(), appID)
		require.NoError(t, err)
		assert.Equal(t, domain.AgentRunStatusRunning, run.Status)
		<-asked

		_, err = svc.StartPreparation(context.Background(), appID)
		require.ErrorIs(t, err, ErrPreparationInProgress, "one run per application at a time")

		require.NoError(t, svc.StopPreparation(run.ID))
		svc.Wait()

		finished, err := runRepo.GetByID(context.Background(), run.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.AgentRunStatusBlocked, finished.Status)
		assert.Equal(t, "you stopped this run", finished.Error)
		assert.Equal(t, domain.ApplicationStateFailed, appRepo.applications[appID].State)
		assert.False(t, worker.closed, "the window stays open for the user")

		assert.ErrorIs(t, svc.StopPreparation(run.ID), ErrRunNotInProgress)
	})

	t.Run("a run in progress when JobPilot stops says so", func(t *testing.T) {
		t.Parallel()
		svc, _, runRepo, worker, appID := newWaitingService(t, time.Minute)
		daemon, stopDaemon := context.WithCancel(context.Background())
		svc.Start(daemon)
		asked := make(chan struct{})
		worker.detect = alwaysChecking(asked)

		run, err := svc.StartPreparation(context.Background(), appID)
		require.NoError(t, err)
		<-asked
		stopDaemon()
		svc.Wait()

		finished, err := runRepo.GetByID(context.Background(), run.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.AgentRunStatusBlocked, finished.Status)
		assert.Equal(t, "JobPilot stopped", finished.Error)
	})
}
