package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"strconv"
	"testing"

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
}

func newFakeJobRepository() *fakeJobRepository {
	return &fakeJobRepository{jobs: map[string]*domain.Job{}, byURL: map[string]*domain.Job{}}
}

func (f *fakeJobRepository) Save(_ context.Context, job *domain.Job) error {
	if job.ID == "" {
		job.ID = fmt.Sprintf("job-%d", len(f.jobs)+1)
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

func (f *fakeJobRepository) List(_ context.Context, _, _ int) ([]*domain.Job, error) {
	var out []*domain.Job
	for _, j := range f.jobs {
		out = append(out, j)
	}
	return out, nil
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
}

func (f *fakeJobSource) ID() string   { return "fake" }
func (f *fakeJobSource) Name() string { return "Fake Source" }
func (f *fakeJobSource) Capabilities() ports.SourceCapabilities {
	return ports.SourceCapabilities{CanSearch: true, CanGetJob: true}
}
func (f *fakeJobSource) Search(_ context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	return f.searchResult, nil
}
func (f *fakeJobSource) GetJob(_ context.Context, _ string) (*domain.Job, error) {
	return f.getJobResult, nil
}

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
	llmProv := &fakeLLMProvider{response: ports.Response{Content: `{"score":80,"confidence":0.9,"rationale":"good fit"}`, Model: "fake-model"}}
	source := &fakeJobSource{}

	svc := Services{
		Candidates: candRepo,
		Onboarding: services.NewOnboardingService(candRepo, services.NewResumeService(resRepo, llmProv), llmProv, nil),
		Profile:    services.NewProfileService(candRepo),
		Resume:     services.NewResumeService(resRepo, llmProv),
		Discovery:  services.NewJobDiscoveryService(jobRepo, []ports.JobSource{source}),
		Matching:   services.NewMatchingService(matchRepo, jobRepo, candRepo, resRepo, llmProv),
		Settings:   services.NewSettingsService(settingsRepo),
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

func TestClient_ProfileRoundTrip(t *testing.T) {
	client, candRepo, _ := testServer(t)
	ctx := context.Background()

	profile := domain.Candidate{Name: "Ada Lovelace"}
	require.NoError(t, client.InitializeProfile(ctx, &profile))
	require.NotEmpty(t, profile.ID)

	got, err := client.GetProfile(ctx)
	require.NoError(t, err)
	require.Equal(t, "Ada Lovelace", got.Name)

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

	profile := domain.Candidate{Name: "Grace Hopper"}
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
