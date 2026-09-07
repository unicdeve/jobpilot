package services

import (
	"context"
	"fmt"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fakeCandidateRepository is an in-memory ports.CandidateRepository for
// tests. Error fields, when set, make the corresponding method fail.
type fakeCandidateRepository struct {
	candidates map[string]*domain.Candidate
	saveErr    error
	getErr     error
	updateErr  error
	deleteErr  error
}

func newFakeCandidateRepository() *fakeCandidateRepository {
	return &fakeCandidateRepository{candidates: map[string]*domain.Candidate{}}
}

func (f *fakeCandidateRepository) Save(_ context.Context, c *domain.Candidate) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if c.ID == "" {
		c.ID = fmt.Sprintf("cand-%d", len(f.candidates)+1)
	}
	f.candidates[c.ID] = c
	return nil
}

func (f *fakeCandidateRepository) GetByID(_ context.Context, id string) (*domain.Candidate, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	c, ok := f.candidates[id]
	if !ok {
		return nil, fmt.Errorf("candidate not found: %s", id)
	}
	return c, nil
}

func (f *fakeCandidateRepository) Update(_ context.Context, c *domain.Candidate) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.candidates[c.ID] = c
	return nil
}

func (f *fakeCandidateRepository) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.candidates, id)
	return nil
}

func (f *fakeCandidateRepository) GetFirstCandidate(_ context.Context) (*domain.Candidate, error) {
	for _, c := range f.candidates {
		return c, nil
	}
	return nil, fmt.Errorf("no candidate profile found")
}

// fakeResumeRepository is an in-memory ports.ResumeRepository for tests.
type fakeResumeRepository struct {
	resumes   map[string]*domain.Resume
	saveErr   error
	getErr    error
	listErr   error
	deleteErr error
}

func newFakeResumeRepository() *fakeResumeRepository {
	return &fakeResumeRepository{resumes: map[string]*domain.Resume{}}
}

func (f *fakeResumeRepository) Save(_ context.Context, r *domain.Resume) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if r.ID == "" {
		r.ID = fmt.Sprintf("resume-%d", len(f.resumes)+1)
	}
	f.resumes[r.ID] = r
	return nil
}

func (f *fakeResumeRepository) GetByID(_ context.Context, id string) (*domain.Resume, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	r, ok := f.resumes[id]
	if !ok {
		return nil, fmt.Errorf("resume not found: %s", id)
	}
	return r, nil
}

func (f *fakeResumeRepository) GetByCandidateID(_ context.Context, candidateID string) ([]*domain.Resume, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []*domain.Resume
	for _, r := range f.resumes {
		if r.CandidateID == candidateID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeResumeRepository) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.resumes, id)
	return nil
}

// fakeJobRepository is an in-memory ports.JobRepository for tests.
type fakeJobRepository struct {
	jobs        map[string]*domain.Job
	byURL       map[string]*domain.Job
	saveErr     error
	getByURLErr error
	listErr     error
	saveCalls   int
}

func newFakeJobRepository() *fakeJobRepository {
	return &fakeJobRepository{jobs: map[string]*domain.Job{}, byURL: map[string]*domain.Job{}}
}

func (f *fakeJobRepository) Save(_ context.Context, job *domain.Job) error {
	f.saveCalls++
	if f.saveErr != nil {
		return f.saveErr
	}
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
	if f.getByURLErr != nil {
		return nil, f.getByURLErr
	}
	j, ok := f.byURL[url]
	if !ok {
		return nil, nil
	}
	return j, nil
}

func (f *fakeJobRepository) List(_ context.Context, _, _ int) ([]*domain.Job, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
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

// fakeMatchRepository is an in-memory ports.MatchRepository for tests.
type fakeMatchRepository struct {
	matches map[string]*domain.JobMatch
	saveErr error
}

func newFakeMatchRepository() *fakeMatchRepository {
	return &fakeMatchRepository{matches: map[string]*domain.JobMatch{}}
}

func (f *fakeMatchRepository) Save(_ context.Context, m *domain.JobMatch) error {
	if f.saveErr != nil {
		return f.saveErr
	}
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

// fakeSettingsRepository is an in-memory ports.SettingsRepository for tests.
type fakeSettingsRepository struct {
	values map[string]string
	getErr error
	setErr error
}

func newFakeSettingsRepository() *fakeSettingsRepository {
	return &fakeSettingsRepository{values: map[string]string{}}
}

func (f *fakeSettingsRepository) Get(_ context.Context, key string) (string, bool, error) {
	if f.getErr != nil {
		return "", false, f.getErr
	}
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeSettingsRepository) Set(_ context.Context, key, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.values[key] = value
	return nil
}

// fakeLLMProvider is a ports.LLMProvider test double. It records the last
// request it received so tests can assert on what was actually sent (e.g.
// untrusted-content wrapping) without depending on unexported helpers.
type fakeLLMProvider struct {
	response        ports.Response
	generateErr     error
	connectivityErr error
	providerName    string
	lastRequest     ports.Request
	generateCalls   int
}

func (f *fakeLLMProvider) Generate(_ context.Context, req ports.Request) (ports.Response, error) {
	f.lastRequest = req
	f.generateCalls++
	if f.generateErr != nil {
		return ports.Response{}, f.generateErr
	}
	return f.response, nil
}

func (f *fakeLLMProvider) TestConnectivity(_ context.Context) error {
	return f.connectivityErr
}

func (f *fakeLLMProvider) Provider() string {
	if f.providerName == "" {
		return "fake"
	}
	return f.providerName
}

// fakeJobSource is a ports.JobSource test double.
type fakeJobSource struct {
	id           string
	capabilities ports.SourceCapabilities
	searchResult []domain.Job
	searchErr    error
	getJobResult *domain.Job
	getJobErr    error
}

func (f *fakeJobSource) ID() string { return f.id }

func (f *fakeJobSource) Name() string { return f.id }

func (f *fakeJobSource) Capabilities() ports.SourceCapabilities { return f.capabilities }

func (f *fakeJobSource) Search(_ context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchResult, nil
}

func (f *fakeJobSource) GetJob(_ context.Context, _ string) (*domain.Job, error) {
	if f.getJobErr != nil {
		return nil, f.getJobErr
	}
	return f.getJobResult, nil
}
