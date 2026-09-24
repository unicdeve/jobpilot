package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// Client is a thin HTTP client for the daemon's local API. It hides the
// transport (loopback HTTP) behind method calls that mirror the core
// services, so callers (currently cmd/jobpilot-cli) don't need to know
// they're talking to a separate process.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a Client targeting a daemon listening on port on
// localhost.
func NewClient(port int) *Client {
	return &Client{
		baseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Health reports whether the daemon is reachable and responding.
func (c *Client) Health(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/v1/health", nil, nil)
	return err
}

// SetupEnvironment initializes the daemon's local storage and database.
func (c *Client) SetupEnvironment(ctx context.Context) (string, error) {
	var resp struct {
		DataDir string `json:"data_dir"`
	}
	_, err := c.do(ctx, http.MethodPost, "/v1/onboarding/environment", nil, &resp)
	return resp.DataDir, err
}

// ConfigureLLM checks connectivity to the daemon's configured LLM provider.
func (c *Client) ConfigureLLM(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodPost, "/v1/onboarding/llm-check", nil, nil)
	return err
}

// InitializeProfile creates the initial candidate profile. It mutates
// profile in place with the server-assigned fields (e.g. ID), matching the
// in-process OnboardingService.InitializeProfile behavior it replaces.
func (c *Client) InitializeProfile(ctx context.Context, profile *domain.Candidate) error {
	_, err := c.do(ctx, http.MethodPost, "/v1/onboarding/profile", profile, profile)
	return err
}

// GetProfile returns the local candidate profile.
func (c *Client) GetProfile(ctx context.Context) (*domain.Candidate, error) {
	var profile domain.Candidate
	_, err := c.do(ctx, http.MethodGet, "/v1/profile", nil, &profile)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// UpdateProfile persists changes to the candidate profile.
func (c *Client) UpdateProfile(ctx context.Context, profile *domain.Candidate) error {
	_, err := c.do(ctx, http.MethodPut, "/v1/profile", profile, profile)
	return err
}

// ImportResume converts a local file at filePath to markdown and stores it
// against candidateID.
func (c *Client) ImportResume(ctx context.Context, candidateID, filePath string) (*domain.Resume, error) {
	req := importResumeRequest{CandidateID: candidateID, FilePath: filePath}
	var resume domain.Resume
	_, err := c.do(ctx, http.MethodPost, "/v1/resumes", req, &resume)
	if err != nil {
		return nil, err
	}
	return &resume, nil
}

// ListResumes returns all resumes belonging to candidateID.
func (c *Client) ListResumes(ctx context.Context, candidateID string) ([]*domain.Resume, error) {
	var resumes []*domain.Resume
	path := "/v1/resumes?" + url.Values{"candidate_id": {candidateID}}.Encode()
	_, err := c.do(ctx, http.MethodGet, path, nil, &resumes)
	return resumes, err
}

// DeleteResume removes a resume by ID.
func (c *Client) DeleteResume(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/resumes/"+url.PathEscape(id), nil, nil)
	return err
}

// ProposeProfileDraft asks the daemon's LLM to draft profile fields from
// resumeMarkdown, for the caller to review before applying.
func (c *Client) ProposeProfileDraft(ctx context.Context, resumeMarkdown string) (*domain.ProfileDraft, error) {
	req := proposeDraftRequest{ResumeMarkdown: resumeMarkdown}
	var draft domain.ProfileDraft
	_, err := c.do(ctx, http.MethodPost, "/v1/resumes/propose-draft", req, &draft)
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

// ListJobs returns a page of jobs already stored locally, most recently
// discovered first, optionally filtered by keywords. It reads local history
// only and contacts no job source, so unlike SearchJobs it works offline. A
// limit of zero means the daemon's default page size.
func (c *Client) ListJobs(ctx context.Context, keywords string, limit, offset int) (*domain.JobPage, error) {
	query := url.Values{}
	if keywords != "" {
		query.Set("q", keywords)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}

	path := "/v1/jobs"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var page domain.JobPage
	if _, err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// ListSavedSearches returns the user's named searches, default first.
func (c *Client) ListSavedSearches(ctx context.Context) ([]*domain.SavedSearch, error) {
	var searches []*domain.SavedSearch
	_, err := c.do(ctx, http.MethodGet, "/v1/searches", nil, &searches)
	return searches, err
}

// GetSavedSearch returns one named search by ID.
func (c *Client) GetSavedSearch(ctx context.Context, id string) (*domain.SavedSearch, error) {
	var search domain.SavedSearch
	if _, err := c.do(ctx, http.MethodGet, "/v1/searches/"+url.PathEscape(id), nil, &search); err != nil {
		return nil, err
	}
	return &search, nil
}

// CreateSavedSearch stores a new named search.
func (c *Client) CreateSavedSearch(ctx context.Context, name string, criteria domain.SearchCriteria) (*domain.SavedSearch, error) {
	req := savedSearchRequest{Name: name, Criteria: criteria}
	var search domain.SavedSearch
	if _, err := c.do(ctx, http.MethodPost, "/v1/searches", req, &search); err != nil {
		return nil, err
	}
	return &search, nil
}

// UpdateSavedSearch replaces a saved search's name and criteria.
func (c *Client) UpdateSavedSearch(ctx context.Context, id, name string, criteria domain.SearchCriteria) (*domain.SavedSearch, error) {
	req := savedSearchRequest{Name: name, Criteria: criteria}
	var search domain.SavedSearch
	if _, err := c.do(ctx, http.MethodPut, "/v1/searches/"+url.PathEscape(id), req, &search); err != nil {
		return nil, err
	}
	return &search, nil
}

// DeleteSavedSearch removes a named search.
func (c *Client) DeleteSavedSearch(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/searches/"+url.PathEscape(id), nil, nil)
	return err
}

// SetDefaultSavedSearch promotes a named search to be the one discovery runs
// when no other is named.
func (c *Client) SetDefaultSavedSearch(ctx context.Context, id string) (*domain.SavedSearch, error) {
	var search domain.SavedSearch
	if _, err := c.do(ctx, http.MethodPost, "/v1/searches/"+url.PathEscape(id)+"/default", nil, &search); err != nil {
		return nil, err
	}
	return &search, nil
}

// RunSavedSearch runs discovery with a saved search's criteria.
func (c *Client) RunSavedSearch(ctx context.Context, id string) ([]*domain.Job, error) {
	var jobs []*domain.Job
	_, err := c.do(ctx, http.MethodPost, "/v1/searches/"+url.PathEscape(id)+"/run", nil, &jobs)
	return jobs, err
}

// GetJob returns one stored job by ID.
func (c *Client) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	var job domain.Job
	if _, err := c.do(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(id), nil, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// SearchJobs runs a job search against criteria.
func (c *Client) SearchJobs(ctx context.Context, criteria domain.SearchCriteria) ([]*domain.Job, error) {
	var jobs []*domain.Job
	_, err := c.do(ctx, http.MethodPost, "/v1/jobs/search", criteria, &jobs)
	return jobs, err
}

// ImportJob imports a specific job by URL.
func (c *Client) ImportJob(ctx context.Context, jobURL string) (*domain.Job, error) {
	req := importJobRequest{URL: jobURL}
	var job domain.Job
	_, err := c.do(ctx, http.MethodPost, "/v1/jobs/import", req, &job)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// AnalyzeMatch runs a match analysis for candidateID against jobID.
func (c *Client) AnalyzeMatch(ctx context.Context, candidateID, jobID string) (*domain.JobMatch, error) {
	req := analyzeMatchRequest{CandidateID: candidateID, JobID: jobID}
	var match domain.JobMatch
	_, err := c.do(ctx, http.MethodPost, "/v1/matches/analyze", req, &match)
	if err != nil {
		return nil, err
	}
	return &match, nil
}

// ListMatches returns all match analyses for candidateID.
func (c *Client) ListMatches(ctx context.Context, candidateID string) ([]*domain.JobMatch, error) {
	var matches []*domain.JobMatch
	path := "/v1/matches?" + url.Values{"candidate_id": {candidateID}}.Encode()
	_, err := c.do(ctx, http.MethodGet, path, nil, &matches)
	return matches, err
}

// GetLLMSettings returns the daemon's configured LLM backend.
func (c *Client) GetLLMSettings(ctx context.Context) (*domain.LLMSettings, error) {
	var settings domain.LLMSettings
	_, err := c.do(ctx, http.MethodGet, "/v1/settings/llm", nil, &settings)
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

// SaveLLMSettingsResult is the outcome of saving new LLM settings: the save
// itself either succeeds or fails, but a subsequent connectivity check
// against the new backend is reported separately and never fails the save.
type SaveLLMSettingsResult struct {
	Settings          domain.LLMSettings
	ConnectivityError string
}

// SaveLLMSettings persists new LLM settings and reports whether the daemon
// could connect to the newly configured backend.
func (c *Client) SaveLLMSettings(ctx context.Context, settings domain.LLMSettings) (*SaveLLMSettingsResult, error) {
	var resp saveLLMSettingsResponse
	_, err := c.do(ctx, http.MethodPut, "/v1/settings/llm", settings, &resp)
	if err != nil {
		return nil, err
	}
	return &SaveLLMSettingsResult{Settings: resp.Settings, ConnectivityError: resp.ConnectivityError}, nil
}

// CreateApplication starts tracking a candidate's application to a job.
func (c *Client) CreateApplication(ctx context.Context, jobID, candidateID, resumeID string, mode domain.ApplicationMode) (*domain.Application, error) {
	req := createApplicationRequest{JobID: jobID, CandidateID: candidateID, ResumeID: resumeID, Mode: mode}
	var app domain.Application
	_, err := c.do(ctx, http.MethodPost, "/v1/applications", req, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// ListApplications returns every application for candidateID.
func (c *Client) ListApplications(ctx context.Context, candidateID string) ([]*domain.Application, error) {
	var apps []*domain.Application
	path := "/v1/applications?" + url.Values{"candidate_id": {candidateID}}.Encode()
	_, err := c.do(ctx, http.MethodGet, path, nil, &apps)
	return apps, err
}

// GetApplication retrieves an application by ID.
func (c *Client) GetApplication(ctx context.Context, id string) (*domain.Application, error) {
	var app domain.Application
	_, err := c.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(id), nil, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// TransitionApplication moves an application to a new state.
func (c *Client) TransitionApplication(ctx context.Context, id string, toState domain.ApplicationState, reason string) (*domain.Application, error) {
	req := transitionApplicationRequest{ToState: toState, Reason: reason}
	var app domain.Application
	_, err := c.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(id)+"/transition", req, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// ListAllowedTransitions returns the states the application may move to
// next, as the daemon's own state machine sees them.
func (c *Client) ListAllowedTransitions(ctx context.Context, id string) ([]domain.ApplicationState, error) {
	var states []domain.ApplicationState
	_, err := c.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(id)+"/transitions", nil, &states)
	return states, err
}

// ListApplicationHistory returns the durable state-transition history for an
// application.
func (c *Client) ListApplicationHistory(ctx context.Context, id string) ([]*domain.ApplicationStateTransition, error) {
	var history []*domain.ApplicationStateTransition
	_, err := c.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(id)+"/history", nil, &history)
	return history, err
}

// GenerateCoverLetter drafts a new cover letter version for an application
// (also used to regenerate one).
func (c *Client) GenerateCoverLetter(ctx context.Context, applicationID string) (*domain.CoverLetter, error) {
	var coverLetter domain.CoverLetter
	_, err := c.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(applicationID)+"/cover-letter", nil, &coverLetter)
	if err != nil {
		return nil, err
	}
	return &coverLetter, nil
}

// ListCoverLetterVersions returns every cover letter version generated for
// an application.
func (c *Client) ListCoverLetterVersions(ctx context.Context, applicationID string) ([]*domain.CoverLetter, error) {
	var versions []*domain.CoverLetter
	_, err := c.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(applicationID)+"/cover-letters", nil, &versions)
	return versions, err
}

// EditCoverLetter persists a user's manual edit to an existing cover letter
// version.
func (c *Client) EditCoverLetter(ctx context.Context, coverLetterID, content string) (*domain.CoverLetter, error) {
	req := editCoverLetterRequest{Content: content}
	var coverLetter domain.CoverLetter
	_, err := c.do(ctx, http.MethodPut, "/v1/cover-letters/"+url.PathEscape(coverLetterID), req, &coverLetter)
	if err != nil {
		return nil, err
	}
	return &coverLetter, nil
}

// AddQuestion registers a new question on an application.
func (c *Client) AddQuestion(ctx context.Context, applicationID, label string, questionType domain.QuestionType, required bool) (*domain.ApplicationQuestion, error) {
	req := addQuestionRequest{Label: label, Type: questionType, Required: required}
	var question domain.ApplicationQuestion
	_, err := c.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(applicationID)+"/questions", req, &question)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// ListQuestions returns every question on an application.
func (c *Client) ListQuestions(ctx context.Context, applicationID string) ([]*domain.ApplicationQuestion, error) {
	var questions []*domain.ApplicationQuestion
	_, err := c.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(applicationID)+"/questions", nil, &questions)
	return questions, err
}

// ResolveQuestion attempts to answer a question via the resolver chain.
func (c *Client) ResolveQuestion(ctx context.Context, questionID string) (*domain.ApplicationQuestion, error) {
	var question domain.ApplicationQuestion
	_, err := c.do(ctx, http.MethodPost, "/v1/questions/"+url.PathEscape(questionID)+"/resolve", nil, &question)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// SetQuestionAnswer records a user-supplied answer for a question.
func (c *Client) SetQuestionAnswer(ctx context.Context, questionID, answer string) (*domain.ApplicationQuestion, error) {
	req := setQuestionAnswerRequest{Answer: answer}
	var question domain.ApplicationQuestion
	_, err := c.do(ctx, http.MethodPut, "/v1/questions/"+url.PathEscape(questionID)+"/answer", req, &question)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// PrepareInBrowser navigates to the application's job posting in a real,
// visible browser window, extracts and fills its form, and returns the
// application's state afterward (READY_FOR_REVIEW on success, FAILED with a
// reason recorded in its history/agent runs if blocked). The browser window
// is left open either way for the user to review and submit themselves.
func (c *Client) PrepareInBrowser(ctx context.Context, applicationID string) (*domain.Application, error) {
	var app domain.Application
	_, err := c.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(applicationID)+"/prepare-in-browser", nil, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// ListAgentRuns returns every agent run (with its events) recorded for an
// application, most recent first.
func (c *Client) ListAgentRuns(ctx context.Context, applicationID string) ([]AgentRunWithEvents, error) {
	var runs []AgentRunWithEvents
	_, err := c.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(applicationID)+"/agent-runs", nil, &runs)
	return runs, err
}

// do sends an HTTP request to path with an optional JSON body, decodes a
// JSON response into out (if non-nil), and turns a non-2xx response into a
// Go error carrying the server's message.
func (c *Client) do(ctx context.Context, method, path string, body, out any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to encode request: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach jobpilot daemon: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errBody); err == nil && errBody.Error != "" {
			return resp, fmt.Errorf("%s", errBody.Error)
		}
		return resp, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp, nil
}

// ListScouts returns every configured scout.
func (c *Client) ListScouts(ctx context.Context) ([]*domain.Scout, error) {
	var scouts []*domain.Scout
	_, err := c.do(ctx, http.MethodGet, "/v1/scouts", nil, &scouts)
	return scouts, err
}

// GetScout returns one scout.
func (c *Client) GetScout(ctx context.Context, id string) (*domain.Scout, error) {
	var scout domain.Scout
	_, err := c.do(ctx, http.MethodGet, "/v1/scouts/"+url.PathEscape(id), nil, &scout)
	return &scout, err
}

// CreateScout stores a new scout.
func (c *Client) CreateScout(ctx context.Context, scout *domain.Scout) (*domain.Scout, error) {
	var created domain.Scout
	_, err := c.do(ctx, http.MethodPost, "/v1/scouts", scoutRequestFrom(scout), &created)
	return &created, err
}

// UpdateScout persists changes to a scout.
func (c *Client) UpdateScout(ctx context.Context, scout *domain.Scout) (*domain.Scout, error) {
	var updated domain.Scout
	_, err := c.do(ctx, http.MethodPut, "/v1/scouts/"+url.PathEscape(scout.ID),
		scoutRequestFrom(scout), &updated)
	return &updated, err
}

// DeleteScout removes a scout.
func (c *Client) DeleteScout(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/scouts/"+url.PathEscape(id), nil, nil)
	return err
}

// RunScout runs a scout once and returns what it did.
//
// A run that fails partway still returns a report, because the
// applications it created before failing are real and the user can act on
// them (AC-006).
func (c *Client) RunScout(ctx context.Context, id string) (*domain.ScoutRunReport, error) {
	var report domain.ScoutRunReport
	_, err := c.do(ctx, http.MethodPost, "/v1/scouts/"+url.PathEscape(id)+"/run", nil, &report)
	return &report, err
}

// ListScoutRuns returns recent scout runs, newest first.
func (c *Client) ListScoutRuns(ctx context.Context, limit int) ([]*domain.AgentRun, error) {
	var runs []*domain.AgentRun
	path := "/v1/scout-runs"
	if limit > 0 {
		path += "?" + url.Values{"limit": {strconv.Itoa(limit)}}.Encode()
	}
	_, err := c.do(ctx, http.MethodGet, path, nil, &runs)
	return runs, err
}

// ListScoutRunEvents returns what one run did, in order.
func (c *Client) ListScoutRunEvents(ctx context.Context, runID string) ([]*domain.AgentEvent, error) {
	var events []*domain.AgentEvent
	_, err := c.do(ctx, http.MethodGet, "/v1/scout-runs/"+url.PathEscape(runID)+"/events", nil, &events)
	return events, err
}

// scoutRequestFrom converts a scout to the wire shape, which carries only
// the fields a caller owns — the server assigns identity and timestamps.
func scoutRequestFrom(scout *domain.Scout) scoutRequest {
	return scoutRequest{
		Name:                  scout.Name,
		SavedSearchID:         scout.SavedSearchID,
		MinMatchScore:         scout.MinMatchScore,
		MaxApplicationsPerRun: scout.MaxApplicationsPerRun,
		Source:                scout.Source,
		IsEnabled:             scout.IsEnabled,
	}
}
