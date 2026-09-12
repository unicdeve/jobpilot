package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// --- in-memory port fakes, local to this package's tests ---

type fakeCandidateRepository struct {
	candidates map[string]*domain.Candidate
}

func newFakeCandidateRepository() *fakeCandidateRepository {
	return &fakeCandidateRepository{candidates: map[string]*domain.Candidate{}}
}

func (f *fakeCandidateRepository) Save(_ context.Context, c *domain.Candidate) error {
	if c.ID == "" {
		c.ID = fmt.Sprintf("cand-%d", len(f.candidates)+1)
	}
	f.candidates[c.ID] = c
	return nil
}

func (f *fakeCandidateRepository) GetByID(_ context.Context, id string) (*domain.Candidate, error) {
	c, ok := f.candidates[id]
	if !ok {
		return nil, fmt.Errorf("candidate not found: %s", id)
	}
	return c, nil
}

func (f *fakeCandidateRepository) Update(_ context.Context, c *domain.Candidate) error {
	f.candidates[c.ID] = c
	return nil
}

func (f *fakeCandidateRepository) Delete(_ context.Context, id string) error {
	delete(f.candidates, id)
	return nil
}

func (f *fakeCandidateRepository) GetFirstCandidate(_ context.Context) (*domain.Candidate, error) {
	for _, c := range f.candidates {
		return c, nil
	}
	return nil, fmt.Errorf("no candidate profile found")
}

type fakeResumeRepository struct {
	resumes map[string]*domain.Resume
}

func newFakeResumeRepository() *fakeResumeRepository {
	return &fakeResumeRepository{resumes: map[string]*domain.Resume{}}
}

func (f *fakeResumeRepository) Save(_ context.Context, r *domain.Resume) error {
	if r.ID == "" {
		r.ID = fmt.Sprintf("resume-%d", len(f.resumes)+1)
	}
	f.resumes[r.ID] = r
	return nil
}

func (f *fakeResumeRepository) GetByID(_ context.Context, id string) (*domain.Resume, error) {
	r, ok := f.resumes[id]
	if !ok {
		return nil, fmt.Errorf("resume not found: %s", id)
	}
	return r, nil
}

func (f *fakeResumeRepository) GetByCandidateID(_ context.Context, candidateID string) ([]*domain.Resume, error) {
	var out []*domain.Resume
	for _, r := range f.resumes {
		if r.CandidateID == candidateID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeResumeRepository) Delete(_ context.Context, id string) error {
	delete(f.resumes, id)
	return nil
}

type fakeJobRepository struct {
	jobs  map[string]*domain.Job
	byURL map[string]*domain.Job
	order []string // insertion order, so List is deterministic
}

func newFakeJobRepository() *fakeJobRepository {
	return &fakeJobRepository{jobs: map[string]*domain.Job{}, byURL: map[string]*domain.Job{}}
}

func (f *fakeJobRepository) Save(_ context.Context, job *domain.Job) error {
	if job.ID == "" {
		job.ID = fmt.Sprintf("job-%d", len(f.jobs)+1)
	}
	if _, exists := f.jobs[job.ID]; !exists {
		f.order = append(f.order, job.ID)
	}
	f.jobs[job.ID] = job
	f.byURL[job.CanonicalURL] = job
	return nil
}

func (f *fakeJobRepository) GetByID(_ context.Context, id string) (*domain.Job, error) {
	j, ok := f.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return j, nil
}

func (f *fakeJobRepository) GetByCanonicalURL(_ context.Context, url string) (*domain.Job, error) {
	return f.byURL[url], nil
}

// matching returns saved jobs whose title or company contains keywords, in
// insertion order, mirroring the SQLite repository's substring rule.
func (f *fakeJobRepository) matching(keywords string) []*domain.Job {
	keywords = strings.ToLower(strings.TrimSpace(keywords))

	out := make([]*domain.Job, 0, len(f.order))
	for _, id := range f.order {
		j := f.jobs[id]
		if keywords != "" &&
			!strings.Contains(strings.ToLower(j.Title), keywords) &&
			!strings.Contains(strings.ToLower(j.CompanyName), keywords) {
			continue
		}
		out = append(out, j)
	}
	return out
}

func (f *fakeJobRepository) List(_ context.Context, keywords string, limit, offset int) ([]*domain.Job, error) {
	matched := f.matching(keywords)
	if offset >= len(matched) {
		return []*domain.Job{}, nil
	}
	return matched[offset:min(offset+limit, len(matched))], nil
}

func (f *fakeJobRepository) Count(_ context.Context, keywords string) (int, error) {
	return len(f.matching(keywords)), nil
}

func (f *fakeJobRepository) Delete(_ context.Context, id string) error {
	delete(f.jobs, id)
	return nil
}

type fakeMatchRepository struct {
	matches map[string]*domain.JobMatch
}

func newFakeMatchRepository() *fakeMatchRepository {
	return &fakeMatchRepository{matches: map[string]*domain.JobMatch{}}
}

func (f *fakeMatchRepository) Save(_ context.Context, m *domain.JobMatch) error {
	if m.ID == "" {
		m.ID = fmt.Sprintf("match-%d", len(f.matches)+1)
	}
	f.matches[m.ID] = m
	return nil
}

func (f *fakeMatchRepository) GetByID(_ context.Context, id string) (*domain.JobMatch, error) {
	m, ok := f.matches[id]
	if !ok {
		return nil, fmt.Errorf("match not found: %s", id)
	}
	return m, nil
}

func (f *fakeMatchRepository) GetByJobID(_ context.Context, jobID string) (*domain.JobMatch, error) {
	for _, m := range f.matches {
		if m.JobID == jobID {
			return m, nil
		}
	}
	return nil, nil
}

func (f *fakeMatchRepository) ListByCandidate(_ context.Context, candidateID string) ([]*domain.JobMatch, error) {
	var out []*domain.JobMatch
	for _, m := range f.matches {
		if m.CandidateID == candidateID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeMatchRepository) Delete(_ context.Context, id string) error {
	delete(f.matches, id)
	return nil
}

type fakeSettingsRepository struct {
	values map[string]string
}

func newFakeSettingsRepository() *fakeSettingsRepository {
	return &fakeSettingsRepository{values: map[string]string{}}
}

func (f *fakeSettingsRepository) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeSettingsRepository) Set(_ context.Context, key, value string) error {
	f.values[key] = value
	return nil
}

type fakeLLMProvider struct {
	response ports.Response
}

func (f *fakeLLMProvider) Generate(_ context.Context, _ ports.Request) (ports.Response, error) {
	return f.response, nil
}

func (f *fakeLLMProvider) TestConnectivity(_ context.Context) error { return nil }

func (f *fakeLLMProvider) Provider() string { return "fake" }

type fakeJobSource struct {
	searchResult []domain.Job
	getJobResult *domain.Job
	searchCalls  int
}

func (f *fakeJobSource) ID() string   { return "fake" }
func (f *fakeJobSource) Name() string { return "Fake Source" }
func (f *fakeJobSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, CanGetJob: true}
}
func (f *fakeJobSource) Search(_ context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	f.searchCalls++
	return f.searchResult, nil
}
func (f *fakeJobSource) GetJob(_ context.Context, _ string) (*domain.Job, error) {
	return f.getJobResult, nil
}

type fakeApplicationRepository struct {
	applications map[string]*domain.Application
	transitions  map[string][]*domain.ApplicationStateTransition
}

func newFakeApplicationRepository() *fakeApplicationRepository {
	return &fakeApplicationRepository{
		applications: map[string]*domain.Application{},
		transitions:  map[string][]*domain.ApplicationStateTransition{},
	}
}

func (f *fakeApplicationRepository) Save(_ context.Context, a *domain.Application) error {
	if a.ID == "" {
		a.ID = fmt.Sprintf("app-%d", len(f.applications)+1)
	}
	f.applications[a.ID] = a
	return nil
}

func (f *fakeApplicationRepository) GetByID(_ context.Context, id string) (*domain.Application, error) {
	a, ok := f.applications[id]
	if !ok {
		return nil, fmt.Errorf("application not found: %s", id)
	}
	return a, nil
}

func (f *fakeApplicationRepository) Update(_ context.Context, a *domain.Application) error {
	f.applications[a.ID] = a
	return nil
}

func (f *fakeApplicationRepository) ListByCandidate(_ context.Context, candidateID string) ([]*domain.Application, error) {
	var out []*domain.Application
	for _, a := range f.applications {
		if a.CandidateID == candidateID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeApplicationRepository) Delete(_ context.Context, id string) error {
	delete(f.applications, id)
	return nil
}

func (f *fakeApplicationRepository) TransitionState(_ context.Context, applicationID string, from, to domain.ApplicationState, reason string) error {
	a, ok := f.applications[applicationID]
	if !ok {
		return fmt.Errorf("application not found: %s", applicationID)
	}
	if a.State != from {
		return fmt.Errorf("application %s is not currently in state %s", applicationID, from)
	}
	a.State = to
	f.transitions[applicationID] = append(f.transitions[applicationID], &domain.ApplicationStateTransition{
		ApplicationID: applicationID, FromState: from, ToState: to, Reason: reason,
	})
	return nil
}

func (f *fakeApplicationRepository) ListStateTransitions(_ context.Context, applicationID string) ([]*domain.ApplicationStateTransition, error) {
	return f.transitions[applicationID], nil
}

type fakeCoverLetterRepository struct {
	letters map[string]*domain.CoverLetter
}

func newFakeCoverLetterRepository() *fakeCoverLetterRepository {
	return &fakeCoverLetterRepository{letters: map[string]*domain.CoverLetter{}}
}

func (f *fakeCoverLetterRepository) Save(_ context.Context, c *domain.CoverLetter) error {
	if c.ID == "" {
		c.ID = fmt.Sprintf("cl-%d", len(f.letters)+1)
	}
	f.letters[c.ID] = c
	return nil
}

func (f *fakeCoverLetterRepository) GetByID(_ context.Context, id string) (*domain.CoverLetter, error) {
	c, ok := f.letters[id]
	if !ok {
		return nil, fmt.Errorf("cover letter not found: %s", id)
	}
	return c, nil
}

func (f *fakeCoverLetterRepository) Update(_ context.Context, c *domain.CoverLetter) error {
	f.letters[c.ID] = c
	return nil
}

func (f *fakeCoverLetterRepository) ListByApplication(_ context.Context, applicationID string) ([]*domain.CoverLetter, error) {
	var out []*domain.CoverLetter
	for _, c := range f.letters {
		if c.ApplicationID == applicationID {
			out = append(out, c)
		}
	}
	return out, nil
}

type fakeApplicationQuestionRepository struct {
	questions map[string]*domain.ApplicationQuestion
}

func newFakeApplicationQuestionRepository() *fakeApplicationQuestionRepository {
	return &fakeApplicationQuestionRepository{questions: map[string]*domain.ApplicationQuestion{}}
}

func (f *fakeApplicationQuestionRepository) Save(_ context.Context, q *domain.ApplicationQuestion) error {
	if q.ID == "" {
		q.ID = fmt.Sprintf("q-%d", len(f.questions)+1)
	}
	f.questions[q.ID] = q
	return nil
}

func (f *fakeApplicationQuestionRepository) GetByID(_ context.Context, id string) (*domain.ApplicationQuestion, error) {
	q, ok := f.questions[id]
	if !ok {
		return nil, fmt.Errorf("application question not found: %s", id)
	}
	return q, nil
}

func (f *fakeApplicationQuestionRepository) Update(_ context.Context, q *domain.ApplicationQuestion) error {
	f.questions[q.ID] = q
	return nil
}

func (f *fakeApplicationQuestionRepository) ListByApplication(_ context.Context, applicationID string) ([]*domain.ApplicationQuestion, error) {
	var out []*domain.ApplicationQuestion
	for _, q := range f.questions {
		if q.ApplicationID == applicationID {
			out = append(out, q)
		}
	}
	return out, nil
}

func (f *fakeApplicationQuestionRepository) Delete(_ context.Context, id string) error {
	delete(f.questions, id)
	return nil
}

// fakeAgentRunRepository is an in-memory ports.AgentRunRepository for
// tests.
type fakeAgentRunRepository struct {
	runs map[string]*domain.AgentRun
}

func newFakeAgentRunRepository() *fakeAgentRunRepository {
	return &fakeAgentRunRepository{runs: map[string]*domain.AgentRun{}}
}

func (f *fakeAgentRunRepository) Save(_ context.Context, run *domain.AgentRun) error {
	if run.ID == "" {
		run.ID = fmt.Sprintf("run-%d", len(f.runs)+1)
	}
	f.runs[run.ID] = run
	return nil
}

func (f *fakeAgentRunRepository) Update(_ context.Context, run *domain.AgentRun) error {
	f.runs[run.ID] = run
	return nil
}

func (f *fakeAgentRunRepository) GetByID(_ context.Context, id string) (*domain.AgentRun, error) {
	run, ok := f.runs[id]
	if !ok {
		return nil, fmt.Errorf("agent run not found: %s", id)
	}
	return run, nil
}

func (f *fakeAgentRunRepository) ListByApplication(_ context.Context, applicationID string) ([]*domain.AgentRun, error) {
	var out []*domain.AgentRun
	for _, r := range f.runs {
		if r.ApplicationID == applicationID {
			out = append(out, r)
		}
	}
	return out, nil
}

// fakeAgentEventRepository is an in-memory ports.AgentEventRepository for
// tests.
type fakeAgentEventRepository struct {
	events []*domain.AgentEvent
}

func newFakeAgentEventRepository() *fakeAgentEventRepository {
	return &fakeAgentEventRepository{}
}

func (f *fakeAgentEventRepository) Save(_ context.Context, event *domain.AgentEvent) error {
	if event.ID == "" {
		event.ID = fmt.Sprintf("event-%d", len(f.events)+1)
	}
	f.events = append(f.events, event)
	return nil
}

func (f *fakeAgentEventRepository) ListByRun(_ context.Context, runID string) ([]*domain.AgentEvent, error) {
	var out []*domain.AgentEvent
	for _, e := range f.events {
		if e.RunID == runID {
			out = append(out, e)
		}
	}
	return out, nil
}

// fakeBrowserWorker is a scriptable ports.BrowserWorker test double.
type fakeBrowserWorker struct {
	fields []ports.FormField
}

func (f *fakeBrowserWorker) Navigate(_ context.Context, _ string) error { return nil }

func (f *fakeBrowserWorker) DetectBlocker(_ context.Context) (string, bool) { return "", false }

func (f *fakeBrowserWorker) ExtractForm(_ context.Context) ([]ports.FormField, error) {
	return f.fields, nil
}

func (f *fakeBrowserWorker) ExtractContent(_ context.Context) (string, error) { return "", nil }

func (f *fakeBrowserWorker) Fill(_ context.Context, _, _ string) error { return nil }

func (f *fakeBrowserWorker) Close(_ context.Context) error { return nil }

// testServer wires a full set of real services over fake ports/adapters,
// serves them via NewHandler on a real loopback listener, and returns a
// Client pointed at it — exercising the client and server together, the
// same way cmd/jobpilot-cli talks to cmd/jobpilot in production.
func testServer(t *testing.T) (*Client, *fakeCandidateRepository, *fakeJobSource) {
	t.Helper()

	candRepo := newFakeCandidateRepository()
	resRepo := newFakeResumeRepository()
	jobRepo := newFakeJobRepository()
	matchRepo := newFakeMatchRepository()
	settingsRepo := newFakeSettingsRepository()
	appRepo := newFakeApplicationRepository()
	coverLetterRepo := newFakeCoverLetterRepository()
	questionRepo := newFakeApplicationQuestionRepository()
	llmProv := &fakeLLMProvider{response: ports.Response{Content: `{"score":80,"confidence":0.9,"rationale":"good fit"}`, Model: "fake-model"}}
	source := &fakeJobSource{}

	appSvc := services.NewApplicationService(appRepo)
	answerSvc := services.NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, resRepo, llmProv)

	svc := Services{
		Candidates:  candRepo,
		Onboarding:  services.NewOnboardingService(candRepo, services.NewResumeService(resRepo, llmProv), llmProv, nil),
		Profile:     services.NewProfileService(candRepo),
		Resume:      services.NewResumeService(resRepo, llmProv),
		Discovery:   services.NewJobDiscoveryService(jobRepo, []ports.JobSource{source}, llmProv, nil),
		Matching:    services.NewMatchingService(matchRepo, jobRepo, candRepo, resRepo, llmProv),
		Settings:    services.NewSettingsService(settingsRepo),
		Application: appSvc,
		CoverLetter: services.NewCoverLetterService(coverLetterRepo, appRepo, jobRepo, candRepo, resRepo, llmProv),
		Answers:     answerSvc,
		Browser: services.NewBrowserWorkerService(appSvc, jobRepo, newFakeAgentRunRepository(), newFakeAgentEventRepository(), answerSvc, func() (ports.BrowserWorker, error) {
			return &fakeBrowserWorker{}, nil
		}),
	}

	ts := httptest.NewServer(NewHandler(svc))
	t.Cleanup(ts.Close)

	_, portStr, err := net.SplitHostPort(ts.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	return NewClient(port), candRepo, source
}

func TestClient_Health(t *testing.T) {
	client, _, _ := testServer(t)
	require.NoError(t, client.Health(context.Background()))
}

func TestCORS_AllowsKnownDesktopOrigin(t *testing.T) {
	ts := httptest.NewServer(NewHandler(Services{}))
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/health", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://localhost:1420")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, "http://localhost:1420", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORS_RejectsUnknownOrigin(t *testing.T) {
	ts := httptest.NewServer(NewHandler(Services{}))
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/health", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "https://evil.example")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestCORS_HandlesPreflight(t *testing.T) {
	ts := httptest.NewServer(NewHandler(Services{}))
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/v1/profile", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "tauri://localhost")
	req.Header.Set("Access-Control-Request-Method", "PUT")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, "tauri://localhost", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestClient_ProfileRoundTrip(t *testing.T) {
	client, candRepo, _ := testServer(t)
	ctx := context.Background()

	profile := domain.Candidate{FirstName: "Ada", LastName: "Lovelace"}
	require.NoError(t, client.InitializeProfile(ctx, &profile))
	require.NotEmpty(t, profile.ID)

	got, err := client.GetProfile(ctx)
	require.NoError(t, err)
	require.Equal(t, "Ada", got.FirstName)
	require.Equal(t, "Lovelace", got.LastName)

	got.Summary = "Pioneering computer programmer"
	require.NoError(t, client.UpdateProfile(ctx, got))

	updated, err := candRepo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "Pioneering computer programmer", updated.Summary)
}

func TestClient_ResumeLifecycle(t *testing.T) {
	client, _, _ := testServer(t)
	ctx := context.Background()

	// ImportResume shells out to markitdown for real file conversion, which
	// is exercised by internal/core/services' own tests; here we only need
	// to verify the HTTP plumbing, so we exercise list/delete against a
	// resume created directly through the fake repo... but the client has
	// no direct repo access, so we instead assert the "no resumes yet" and
	// "not found" paths, which don't require a real file on disk.
	resumes, err := client.ListResumes(ctx, "cand-1")
	require.NoError(t, err)
	require.Empty(t, resumes)

	err = client.DeleteResume(ctx, "does-not-exist")
	require.NoError(t, err, "delete is idempotent at the repository layer used here")
}

func TestClient_JobSearchAndImport(t *testing.T) {
	client, _, source := testServer(t)
	ctx := context.Background()

	source.searchResult = []domain.Job{{CanonicalURL: "https://example.com/job/1", Title: "Engineer", CompanyName: "Acme"}}
	jobs, err := client.SearchJobs(ctx, domain.SearchCriteria{Keywords: "engineer"})
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, "Engineer", jobs[0].Title)

	source.getJobResult = &domain.Job{CanonicalURL: "https://example.com/job/2", Title: "Designer", CompanyName: "Acme"}
	job, err := client.ImportJob(ctx, "https://example.com/job/2")
	require.NoError(t, err)
	require.Equal(t, "Designer", job.Title)
}

func TestClient_MatchAnalyzeAndList(t *testing.T) {
	client, candRepo, source := testServer(t)
	ctx := context.Background()

	profile := domain.Candidate{FirstName: "Grace", LastName: "Hopper"}
	require.NoError(t, candRepo.Save(ctx, &profile))

	source.getJobResult = &domain.Job{CanonicalURL: "https://example.com/job/3", Title: "Compiler Engineer"}
	job, err := client.ImportJob(ctx, "https://example.com/job/3")
	require.NoError(t, err)

	match, err := client.AnalyzeMatch(ctx, profile.ID, job.ID)
	require.NoError(t, err)
	require.Equal(t, 80, match.Score)

	matches, err := client.ListMatches(ctx, profile.ID)
	require.NoError(t, err)
	require.Len(t, matches, 1)
}

func TestClient_LLMSettingsRoundTrip(t *testing.T) {
	client, _, _ := testServer(t)
	ctx := context.Background()

	got, err := client.GetLLMSettings(ctx)
	require.NoError(t, err)
	require.Empty(t, got.Provider, "no settings saved yet")

	result, err := client.SaveLLMSettings(ctx, domain.LLMSettings{
		Provider: "custom",
		BaseURL:  "http://127.0.0.1:1/unreachable",
		Model:    "test-model",
	})
	require.NoError(t, err, "saving must succeed even if the backend can't be reached")
	require.NotEmpty(t, result.ConnectivityError, "an unreachable backend should surface as a warning, not a save failure")

	saved, err := client.GetLLMSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "custom", saved.Provider)
}

func TestClient_NotFoundSurfacesServerError(t *testing.T) {
	client, _, _ := testServer(t)
	_, err := client.GetProfile(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no candidate profile found")
}

func TestClient_ApplicationLifecycle(t *testing.T) {
	client, candRepo, source := testServer(t)
	ctx := context.Background()

	candidate := domain.Candidate{FirstName: "Ada", LastName: "Lovelace", Contact: "ada@example.com"}
	require.NoError(t, candRepo.Save(ctx, &candidate))
	source.getJobResult = &domain.Job{CanonicalURL: "https://acme.example/job-1", Title: "Engineer"}
	job, err := client.ImportJob(ctx, "https://acme.example/job-1")
	require.NoError(t, err)

	app, err := client.CreateApplication(ctx, job.ID, candidate.ID, "", domain.ApplicationModeReview)
	require.NoError(t, err)
	require.Equal(t, domain.ApplicationStateDiscovered, app.State)

	got, err := client.GetApplication(ctx, app.ID)
	require.NoError(t, err)
	require.Equal(t, app.ID, got.ID)

	apps, err := client.ListApplications(ctx, candidate.ID)
	require.NoError(t, err)
	require.Len(t, apps, 1)

	transitioned, err := client.TransitionApplication(ctx, app.ID, domain.ApplicationStateMatched, "auto-matched")
	require.NoError(t, err)
	require.Equal(t, domain.ApplicationStateMatched, transitioned.State)

	_, err = client.TransitionApplication(ctx, app.ID, domain.ApplicationStateSubmitted, "")
	require.Error(t, err, "SUBMITTED must only be reachable from APPROVED")

	history, err := client.ListApplicationHistory(ctx, app.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "auto-matched", history[0].Reason)
}

func TestClient_CoverLetterLifecycle(t *testing.T) {
	client, candRepo, source := testServer(t)
	ctx := context.Background()

	candidate := domain.Candidate{FirstName: "Grace", LastName: "Hopper"}
	require.NoError(t, candRepo.Save(ctx, &candidate))
	source.getJobResult = &domain.Job{CanonicalURL: "https://acme.example/job-2", Title: "Compiler Engineer"}
	job, err := client.ImportJob(ctx, "https://acme.example/job-2")
	require.NoError(t, err)
	app, err := client.CreateApplication(ctx, job.ID, candidate.ID, "", domain.ApplicationModeReview)
	require.NoError(t, err)

	first, err := client.GenerateCoverLetter(ctx, app.ID)
	require.NoError(t, err)
	require.NotEmpty(t, first.Content)

	second, err := client.GenerateCoverLetter(ctx, app.ID)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID, "regenerating must create a new version")

	versions, err := client.ListCoverLetterVersions(ctx, app.ID)
	require.NoError(t, err)
	require.Len(t, versions, 2, "regeneration must not discard the earlier version")

	edited, err := client.EditCoverLetter(ctx, second.ID, "manually edited content")
	require.NoError(t, err)
	require.Equal(t, "manually edited content", edited.Content)
	require.True(t, edited.UserEdited)
}

func TestClient_QuestionResolution(t *testing.T) {
	client, candRepo, source := testServer(t)
	ctx := context.Background()

	candidate := domain.Candidate{FirstName: "Alan", LastName: "Turing", Contact: "alan@example.com"}
	require.NoError(t, candRepo.Save(ctx, &candidate))
	source.getJobResult = &domain.Job{CanonicalURL: "https://acme.example/job-3", Title: "Cryptanalyst"}
	job, err := client.ImportJob(ctx, "https://acme.example/job-3")
	require.NoError(t, err)
	app, err := client.CreateApplication(ctx, job.ID, candidate.ID, "", domain.ApplicationModeReview)
	require.NoError(t, err)

	factual, err := client.AddQuestion(ctx, app.ID, "Email address", domain.QuestionTypeFactual, true)
	require.NoError(t, err)
	resolved, err := client.ResolveQuestion(ctx, factual.ID)
	require.NoError(t, err)
	require.Equal(t, "alan@example.com", resolved.Answer)
	require.Equal(t, domain.AnswerCategoryProfileFact, resolved.Category)

	unknown, err := client.AddQuestion(ctx, app.ID, "What is your driver's license number?", domain.QuestionTypeFactual, false)
	require.NoError(t, err)
	resolvedUnknown, err := client.ResolveQuestion(ctx, unknown.ID)
	require.NoError(t, err)
	require.Empty(t, resolvedUnknown.Answer)
	require.Equal(t, domain.AnswerCategoryNeedsUserInput, resolvedUnknown.Category)

	answered, err := client.SetQuestionAnswer(ctx, unknown.ID, "ABC123")
	require.NoError(t, err)
	require.Equal(t, "ABC123", answered.Answer)
	require.True(t, answered.UserEdited)

	questions, err := client.ListQuestions(ctx, app.ID)
	require.NoError(t, err)
	require.Len(t, questions, 2)
}

func TestClient_PrepareInBrowserLifecycle(t *testing.T) {
	client, candRepo, source := testServer(t)
	ctx := context.Background()

	candidate := domain.Candidate{FirstName: "Katherine", LastName: "Johnson"}
	require.NoError(t, candRepo.Save(ctx, &candidate))
	source.getJobResult = &domain.Job{CanonicalURL: "https://acme.example/job-4", Title: "Mathematician"}
	job, err := client.ImportJob(ctx, "https://acme.example/job-4")
	require.NoError(t, err)
	app, err := client.CreateApplication(ctx, job.ID, candidate.ID, "", domain.ApplicationModeReview)
	require.NoError(t, err)

	_, err = client.TransitionApplication(ctx, app.ID, domain.ApplicationStateMatched, "matched")
	require.NoError(t, err)
	_, err = client.TransitionApplication(ctx, app.ID, domain.ApplicationStatePreparing, "preparing")
	require.NoError(t, err)

	// testServer's fake browser worker extracts no fields by default, which
	// PrepareApplication treats as a blocker — this still exercises the
	// endpoint/JSON contract end to end; the resolver/fill orchestration
	// itself is already covered by internal/core/services/browserworker_test.go.
	_, err = client.PrepareInBrowser(ctx, app.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no recognizable application form fields")

	got, err := client.GetApplication(ctx, app.ID)
	require.NoError(t, err)
	require.Equal(t, domain.ApplicationStateFailed, got.State)

	runs, err := client.ListAgentRuns(ctx, app.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, domain.AgentRunStatusBlocked, runs[0].Run.Status)
	require.NotEmpty(t, runs[0].Events)
}

func TestClient_ListJobs(t *testing.T) {
	client, _, source := testServer(t)
	ctx := context.Background()

	// Discover five jobs so there is stored history to page through. The
	// source is only used here; listing must not touch it again.
	source.searchResult = []domain.Job{
		{CanonicalURL: "https://example.com/job/1", Title: "Backend Engineer", CompanyName: "Acme"},
		{CanonicalURL: "https://example.com/job/2", Title: "Frontend Engineer", CompanyName: "Acme"},
		{CanonicalURL: "https://example.com/job/3", Title: "Platform Engineer", CompanyName: "Acme"},
		{CanonicalURL: "https://example.com/job/4", Title: "Product Designer", CompanyName: "Globex"},
		{CanonicalURL: "https://example.com/job/5", Title: "Data Scientist", CompanyName: "Initech"},
	}
	_, err := client.SearchJobs(ctx, domain.SearchCriteria{})
	require.NoError(t, err)

	t.Run("returns stored jobs and the total", func(t *testing.T) {
		page, err := client.ListJobs(ctx, "", 0, 0)
		require.NoError(t, err)
		assert.Len(t, page.Jobs, 5)
		assert.Equal(t, 5, page.Total)
	})

	t.Run("filters by keywords", func(t *testing.T) {
		page, err := client.ListJobs(ctx, "engineer", 0, 0)
		require.NoError(t, err)
		assert.Len(t, page.Jobs, 3)
		assert.Equal(t, 3, page.Total)
	})

	t.Run("paginates, reporting the unpaginated total", func(t *testing.T) {
		page, err := client.ListJobs(ctx, "", 2, 2)
		require.NoError(t, err)
		require.Len(t, page.Jobs, 2)
		assert.Equal(t, "Platform Engineer", page.Jobs[0].Title)
		assert.Equal(t, 5, page.Total)
	})

	t.Run("an offset past the end is an empty page, not an error", func(t *testing.T) {
		page, err := client.ListJobs(ctx, "", 10, 500)
		require.NoError(t, err)
		assert.Empty(t, page.Jobs)
		assert.Equal(t, 5, page.Total)
	})

	t.Run("does not contact a job source", func(t *testing.T) {
		before := source.searchCalls
		_, err := client.ListJobs(ctx, "", 0, 0)
		require.NoError(t, err)
		assert.Equal(t, before, source.searchCalls)
	})
}

func TestServer_ListJobs_RejectsInvalidPaging(t *testing.T) {
	client, _, _ := testServer(t)

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"non-numeric limit", "?limit=many", `invalid limit "many"`},
		{"non-numeric offset", "?offset=first", `invalid offset "first"`},
		{"negative limit", "?limit=-1", "invalid limit -1"},
		{"negative offset", "?offset=-1", "invalid offset -1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := http.Get(client.baseURL + "/v1/jobs" + tt.query)
			require.NoError(t, err)
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusBadRequest, res.StatusCode)

			var body struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			assert.Contains(t, body.Error, tt.want)
		})
	}
}

func TestServer_ListJobs_EmptyHistory(t *testing.T) {
	client, _, _ := testServer(t)

	page, err := client.ListJobs(context.Background(), "", 0, 0)

	require.NoError(t, err)
	assert.Empty(t, page.Jobs)
	assert.Zero(t, page.Total)
}

func TestClient_GetJob(t *testing.T) {
	client, _, source := testServer(t)
	ctx := context.Background()

	source.searchResult = []domain.Job{
		{CanonicalURL: "https://example.com/job/1", Title: "Backend Engineer", CompanyName: "Acme"},
	}
	discovered, err := client.SearchJobs(ctx, domain.SearchCriteria{})
	require.NoError(t, err)
	require.Len(t, discovered, 1)

	job, err := client.GetJob(ctx, discovered[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "Backend Engineer", job.Title)
	assert.Equal(t, "Acme", job.CompanyName)

	_, err = client.GetJob(ctx, "no-such-job")
	assert.ErrorContains(t, err, "no-such-job")
}
