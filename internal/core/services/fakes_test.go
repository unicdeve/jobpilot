package services

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

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
		return nil, &ports.NotFoundError{Kind: "candidate", ID: id}
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

func (f *fakeCandidateRepository) GetFirstCandidate(_ context.Context) (*domain.Candidate, error) {
	for _, c := range f.candidates {
		return c, nil
	}
	return nil, ports.ErrNoCandidate
}

// fakeResumeRepository is an in-memory ports.ResumeRepository for tests.
type fakeResumeRepository struct {
	resumes map[string]*domain.Resume
	saveErr error
	getErr  error
	listErr error
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
		return nil, &ports.NotFoundError{Kind: "resume", ID: id}
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

// fakeJobRepository is an in-memory ports.JobRepository for tests.
type fakeJobRepository struct {
	jobs        map[string]*domain.Job
	byURL       map[string]*domain.Job
	order       []string // insertion order, so List is deterministic
	saveErr     error
	getByURLErr error
	listErr     error
	countErr    error
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
		return nil, &ports.NotFoundError{Kind: "job", ID: id}
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

// matching returns the saved jobs filter keeps, in insertion order: title
// or company containing the keywords — the same naive, case-insensitive substring rule the
// SQLite repository applies.
func (f *fakeJobRepository) matching(filter domain.JobFilter) []*domain.Job {
	keywords := strings.ToLower(strings.TrimSpace(filter.Keywords))

	out := make([]*domain.Job, 0, len(f.order))
	for _, id := range f.order {
		j := f.jobs[id]
		if keywords != "" &&
			!strings.Contains(strings.ToLower(j.Title), keywords) &&
			!strings.Contains(strings.ToLower(j.CompanyName), keywords) {
			continue
		}
		if filter.SourceID != "" && j.SourceID != filter.SourceID {
			continue
		}
		if filter.RemoteType != "" && j.RemoteType != filter.RemoteType {
			continue
		}
		ruledOut := j.NotInterestedAt != nil
		if (filter.Interest == domain.JobsInterested && ruledOut) || (filter.Interest == domain.JobsNotInterested && !ruledOut) {
			continue
		}
		if posted := j.PostedAt; !filter.PostedSince.IsZero() {
			if posted.IsZero() {
				posted = j.DiscoveredAt
			}
			if posted.Before(filter.PostedSince) {
				continue
			}
		}
		out = append(out, j)
	}
	return out
}

func (f *fakeJobRepository) List(_ context.Context, filter domain.JobFilter, limit, offset int) ([]*domain.Job, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	matched := f.matching(filter)
	if offset >= len(matched) {
		return []*domain.Job{}, nil
	}
	end := min(offset+limit, len(matched))
	return matched[offset:end], nil
}

func (f *fakeJobRepository) Count(_ context.Context, filter domain.JobFilter) (int, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return len(f.matching(filter)), nil
}

func (f *fakeJobRepository) ListUnread(_ context.Context, version, limit int) ([]*domain.Job, error) {
	var jobs []*domain.Job
	for _, job := range f.jobs {
		if job.SignalsVersion < version && len(jobs) < limit {
			copied := *job
			jobs = append(jobs, &copied)
		}
	}
	return jobs, nil
}

func (f *fakeJobRepository) SaveSignals(_ context.Context, job *domain.Job) error {
	for _, stored := range f.jobs {
		if stored.ID == job.ID {
			stored.Workable, stored.Relocation, stored.VisaSponsorship = job.Workable, job.Relocation, job.VisaSponsorship
			stored.SignalsVersion = job.SignalsVersion
			return nil
		}
	}
	return &ports.NotFoundError{Kind: "job", ID: job.ID}
}

func (f *fakeJobRepository) SetNotInterested(_ context.Context, id string, at *time.Time) error {
	j, ok := f.jobs[id]
	if !ok {
		return &ports.NotFoundError{Kind: "job", ID: id}
	}
	j.NotInterestedAt = at
	return nil
}

// fakeSavedSearchRepository is an in-memory ports.SavedSearchRepository for
// tests.
type fakeSavedSearchRepository struct {
	searches map[string]*domain.SavedSearch
	order    []string
	saveErr  error
	listErr  error
}

func newFakeSavedSearchRepository() *fakeSavedSearchRepository {
	return &fakeSavedSearchRepository{searches: map[string]*domain.SavedSearch{}}
}

func (f *fakeSavedSearchRepository) Save(_ context.Context, search *domain.SavedSearch) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if search.ID == "" {
		search.ID = fmt.Sprintf("search-%d", len(f.searches)+1)
	}
	if _, exists := f.searches[search.ID]; !exists {
		f.order = append(f.order, search.ID)
	}
	stored := *search
	if previous, ok := f.searches[search.ID]; ok {
		stored.IsDefault = previous.IsDefault
	}
	f.searches[search.ID] = &stored
	return nil
}

func (f *fakeSavedSearchRepository) GetByID(_ context.Context, id string) (*domain.SavedSearch, error) {
	search, ok := f.searches[id]
	if !ok {
		return nil, &ports.NotFoundError{Kind: "saved search", ID: id}
	}
	return search, nil
}

func (f *fakeSavedSearchRepository) GetByName(_ context.Context, name string) (*domain.SavedSearch, error) {
	for _, id := range f.order {
		if strings.EqualFold(f.searches[id].Name, name) {
			return f.searches[id], nil
		}
	}
	return nil, nil
}

// List mirrors the SQLite ordering: default first, then by name.
func (f *fakeSavedSearchRepository) List(_ context.Context) ([]*domain.SavedSearch, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*domain.SavedSearch, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, f.searches[id])
	}
	slices.SortStableFunc(out, func(a, b *domain.SavedSearch) int {
		if a.IsDefault != b.IsDefault {
			if a.IsDefault {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

func (f *fakeSavedSearchRepository) Delete(ctx context.Context, id string) error {
	search, err := f.GetByID(ctx, id)
	if err != nil {
		return err
	}
	remaining, err := f.List(ctx)
	if err != nil {
		return err
	}
	delete(f.searches, id)
	f.order = slices.DeleteFunc(f.order, func(existing string) bool { return existing == id })
	if search.IsDefault {
		for _, next := range remaining {
			if next.ID != id {
				next.IsDefault = true
				break
			}
		}
	}
	return nil
}

func (f *fakeSavedSearchRepository) SetDefault(ctx context.Context, id string) (*domain.SavedSearch, error) {
	selected, err := f.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, search := range f.searches {
		search.IsDefault = search.ID == id
	}
	return selected, nil
}

func (f *fakeSavedSearchRepository) MarkRun(_ context.Context, id string, at time.Time) error {
	if search, ok := f.searches[id]; ok {
		search.LastRunAt = &at
	}
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
		return nil, &ports.NotFoundError{Kind: "match", ID: id}
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

// fakeApplicationRepository is an in-memory ports.ApplicationRepository for
// tests.
type fakeApplicationRepository struct {
	applications  map[string]*domain.Application
	transitions   map[string][]*domain.ApplicationStateTransition
	saveErr       error
	transitionErr error
	// saveErrAfter makes Save fail once this many rows exist, so a test
	// can fail a run partway and check what survives.
	saveErrAfter int
	saveCalls    int
}

func newFakeApplicationRepository() *fakeApplicationRepository {
	return &fakeApplicationRepository{
		applications: map[string]*domain.Application{},
		transitions:  map[string][]*domain.ApplicationStateTransition{},
	}
}

func (f *fakeApplicationRepository) Save(_ context.Context, a *domain.Application) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saveCalls++
	if f.saveErrAfter > 0 && f.saveCalls > f.saveErrAfter {
		return fmt.Errorf("the database is unavailable")
	}
	if a.ID == "" {
		a.ID = fmt.Sprintf("app-%d", len(f.applications)+1)
	}
	f.applications[a.ID] = a
	return nil
}

func (f *fakeApplicationRepository) GetByID(_ context.Context, id string) (*domain.Application, error) {
	a, ok := f.applications[id]
	if !ok {
		return nil, &ports.NotFoundError{Kind: "application", ID: id}
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

func (f *fakeApplicationRepository) TransitionState(_ context.Context, applicationID string, from, to domain.ApplicationState, reason string) error {
	if f.transitionErr != nil {
		return f.transitionErr
	}
	a, ok := f.applications[applicationID]
	if !ok {
		return &ports.NotFoundError{Kind: "application", ID: applicationID}
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

// fakeCoverLetterRepository is an in-memory ports.CoverLetterRepository for
// tests.
type fakeCoverLetterRepository struct {
	letters map[string]*domain.CoverLetter
	saveErr error
}

func newFakeCoverLetterRepository() *fakeCoverLetterRepository {
	return &fakeCoverLetterRepository{letters: map[string]*domain.CoverLetter{}}
}

func (f *fakeCoverLetterRepository) Save(_ context.Context, c *domain.CoverLetter) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if c.ID == "" {
		c.ID = fmt.Sprintf("cl-%d", len(f.letters)+1)
	}
	f.letters[c.ID] = c
	return nil
}

func (f *fakeCoverLetterRepository) GetByID(_ context.Context, id string) (*domain.CoverLetter, error) {
	c, ok := f.letters[id]
	if !ok {
		return nil, &ports.NotFoundError{Kind: "cover letter", ID: id}
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

// fakeApplicationQuestionRepository is an in-memory
// ports.ApplicationQuestionRepository for tests.
type fakeApplicationQuestionRepository struct {
	questions map[string]*domain.ApplicationQuestion
	// order is the IDs in the order they were saved, for oldest-first
	// listing.
	order []string
	saved int
}

func newFakeApplicationQuestionRepository() *fakeApplicationQuestionRepository {
	return &fakeApplicationQuestionRepository{questions: map[string]*domain.ApplicationQuestion{}}
}

func (f *fakeApplicationQuestionRepository) Save(_ context.Context, q *domain.ApplicationQuestion) error {
	if q.ID == "" {
		f.saved++
		q.ID = fmt.Sprintf("q-%d", f.saved)
	}
	if _, ok := f.questions[q.ID]; !ok {
		f.order = append(f.order, q.ID)
	}
	f.questions[q.ID] = q
	return nil
}

func (f *fakeApplicationQuestionRepository) GetByID(_ context.Context, id string) (*domain.ApplicationQuestion, error) {
	q, ok := f.questions[id]
	if !ok {
		return nil, &ports.NotFoundError{Kind: "application question", ID: id}
	}
	return q, nil
}

func (f *fakeApplicationQuestionRepository) Update(_ context.Context, q *domain.ApplicationQuestion) error {
	f.questions[q.ID] = q
	return nil
}

func (f *fakeApplicationQuestionRepository) ListByApplication(_ context.Context, applicationID string) ([]*domain.ApplicationQuestion, error) {
	out := make([]*domain.ApplicationQuestion, 0)
	for _, id := range f.order {
		if q, ok := f.questions[id]; ok && q.ApplicationID == applicationID {
			out = append(out, q)
		}
	}
	return out, nil
}

func (f *fakeApplicationQuestionRepository) Delete(_ context.Context, id string) error {
	if _, ok := f.questions[id]; !ok {
		return &ports.NotFoundError{Kind: "application question", ID: id}
	}
	delete(f.questions, id)
	return nil
}

// fakeJobSource is a ports.JobSource test double.
type fakeJobSource struct {
	id           string
	capabilities ports.SourceCapabilities
	searchResult []domain.Job
	searchErr    error
	getJobResult *domain.Job
	getJobErr    error
	searchCalls  int
}

func (f *fakeJobSource) ID() string { return f.id }

func (f *fakeJobSource) Name() string { return f.id }

func (f *fakeJobSource) Capabilities() ports.SourceCapabilities { return f.capabilities }

func (f *fakeJobSource) Search(_ context.Context, _ domain.SearchCriteria) ([]domain.Job, error) {
	f.searchCalls++
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

// fakeAgentRunRepository is an in-memory ports.AgentRunRepository for tests.
type fakeAgentRunRepository struct {
	runs    map[string]*domain.AgentRun
	saveErr error
}

func newFakeAgentRunRepository() *fakeAgentRunRepository {
	return &fakeAgentRunRepository{runs: map[string]*domain.AgentRun{}}
}

func (f *fakeAgentRunRepository) Save(_ context.Context, run *domain.AgentRun) error {
	if f.saveErr != nil {
		return f.saveErr
	}
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
		return nil, &ports.NotFoundError{Kind: "agent run", ID: id}
	}
	return run, nil
}

func (f *fakeAgentRunRepository) List(_ context.Context, limit int) ([]*domain.AgentRun, error) {
	if limit <= 0 {
		limit = 50
	}
	out := make([]*domain.AgentRun, 0, len(f.runs))
	for _, run := range f.runs {
		out = append(out, run)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].StartedAt.After(out[b].StartedAt) })
	return out[:min(limit, len(out))], nil
}

// ListSummaries filters and pages the stored runs. It names nothing, since
// the fake has no scouts to join against.
func (f *fakeAgentRunRepository) ListSummaries(_ context.Context, filter domain.AgentRunFilter) (*domain.AgentRunPage, error) {
	var matched []*domain.AgentRunSummary
	for _, run := range f.runs {
		if filter.TaskType != "" && run.TaskType != filter.TaskType {
			continue
		}
		if filter.Status != "" && run.Status != filter.Status {
			continue
		}
		matched = append(matched, &domain.AgentRunSummary{AgentRun: *run})
	}
	sort.Slice(matched, func(a, b int) bool { return matched[a].StartedAt.After(matched[b].StartedAt) })

	page := &domain.AgentRunPage{Runs: []*domain.AgentRunSummary{}, Total: len(matched)}
	if filter.Offset < len(matched) {
		matched = matched[filter.Offset:]
		page.Runs = matched[:min(filter.Limit, len(matched))]
	}
	return page, nil
}

func (f *fakeAgentRunRepository) GetSummary(_ context.Context, id string) (*domain.AgentRunSummary, error) {
	run, ok := f.runs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ports.ErrAgentRunNotFound, id)
	}
	return &domain.AgentRunSummary{AgentRun: *run}, nil
}

func (f *fakeAgentRunRepository) CloseUnfinished(_ context.Context, reason string, at time.Time) (int, error) {
	closed := 0
	for _, run := range f.runs {
		if run.Status.InProgress() {
			run.Status, run.Error, run.CompletedAt = domain.AgentRunStatusFailed, reason, &at
			closed++
		}
	}
	return closed, nil
}

func (f *fakeAgentRunRepository) LastScoutStart(_ context.Context, scoutID string) (*time.Time, error) {
	var last *time.Time
	for _, run := range f.runs {
		if run.ScoutID == scoutID && (last == nil || run.StartedAt.After(*last)) {
			started := run.StartedAt
			last = &started
		}
	}
	return last, nil
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
	navigated         []string
	navigateErr       error
	blockedReason     string
	blocked           bool
	closed            bool
	content           string
	extractContentErr error
}

func (f *fakeBrowserWorker) Navigate(_ context.Context, url string) error {
	f.navigated = append(f.navigated, url)
	return f.navigateErr
}

func (f *fakeBrowserWorker) DetectBlocker(_ context.Context) (string, bool) {
	return f.blockedReason, f.blocked
}

func (f *fakeBrowserWorker) ExtractContent(_ context.Context) (string, error) {
	if f.extractContentErr != nil {
		return "", f.extractContentErr
	}
	return f.content, nil
}

func (f *fakeBrowserWorker) Close(_ context.Context) error {
	f.closed = true
	return nil
}

// fakeDocumentConverter stands in for the PDF and markdown engines.
//
// Its existence is the point of ports.DocumentConverter: before ADR-0016,
// ResumeService built a converter itself, so testing ImportResume meant
// having a real PDF on disk and a WebAssembly engine to read it with.
//
// With markdown unset it echoes what it was given, so a test about what
// reaches a prompt still sees its own input arrive there.
type fakeDocumentConverter struct {
	markdown string
	err      error
	// converted records the paths and HTML passed in, so a test can assert
	// what was handed to conversion.
	converted []string
}

func (f *fakeDocumentConverter) convert(input string) (ports.Document, error) {
	f.converted = append(f.converted, input)
	if f.err != nil {
		return ports.Document{}, f.err
	}
	if f.markdown != "" {
		return ports.Document{Markdown: f.markdown}, nil
	}
	return ports.Document{Markdown: input}, nil
}

func (f *fakeDocumentConverter) ConvertFile(_ context.Context, path string) (ports.Document, error) {
	return f.convert(path)
}

func (f *fakeDocumentConverter) ConvertBytes(_ context.Context, name string, data []byte) (ports.Document, error) {
	return f.convert(name + ":" + string(data))
}

func (f *fakeDocumentConverter) ConvertHTML(_ context.Context, html string) (ports.Document, error) {
	return f.convert(html)
}

// candidatesWith is a candidate repository holding a candidate for each ID.
func candidatesWith(ids ...string) *fakeCandidateRepository {
	repo := newFakeCandidateRepository()
	for _, id := range ids {
		repo.candidates[id] = &domain.Candidate{ID: id}
	}
	return repo
}

// jobsWith is a job repository holding a job for each ID.
func jobsWith(ids ...string) *fakeJobRepository {
	repo := newFakeJobRepository()
	for _, id := range ids {
		repo.jobs[id] = &domain.Job{ID: id}
	}
	return repo
}
