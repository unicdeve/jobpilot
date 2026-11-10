package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// Client is a thin HTTP client for the daemon's local API. It hides the
// transport (loopback HTTP) behind method calls that mirror the core
// services, so callers (currently cmd/jobpilot-cli) don't need to know
// they're talking to a separate process.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	workspaceID string
}

// WithWorkspace returns a client whose data requests are explicitly scoped
// to id. The underlying transport is shared.
func (c *Client) WithWorkspace(id string) *Client {
	scoped := *c
	scoped.workspaceID = id
	return &scoped
}

// ListWorkspaces returns the local workspaces and the installation's active
// workspace identifier.
func (c *Client) ListWorkspaces(ctx context.Context) ([]system.Workspace, string, error) {
	var response struct {
		Workspaces []system.Workspace `json:"workspaces"`
		ActiveID   string             `json:"active_workspace_id"`
	}
	_, err := c.do(ctx, http.MethodGet, "/v1/workspaces", nil, &response)
	return response.Workspaces, response.ActiveID, err
}

// CreateWorkspace creates an empty local workspace.
func (c *Client) CreateWorkspace(ctx context.Context, name string) (*system.Workspace, error) {
	var workspace system.Workspace
	_, err := c.do(ctx, http.MethodPost, "/v1/workspaces", map[string]string{"name": name}, &workspace)
	return &workspace, err
}

// RenameWorkspace changes a workspace's display name.
func (c *Client) RenameWorkspace(ctx context.Context, id, name string) (*system.Workspace, error) {
	var workspace system.Workspace
	path := "/v1/workspaces/" + url.PathEscape(id)
	_, err := c.do(ctx, http.MethodPut, path, map[string]string{"name": name}, &workspace)
	return &workspace, err
}

// ActivateWorkspace changes the workspace used by clients without an
// explicit selection.
func (c *Client) ActivateWorkspace(ctx context.Context, id string) (*system.Workspace, error) {
	var workspace system.Workspace
	path := "/v1/workspaces/" + url.PathEscape(id) + "/activate"
	_, err := c.do(ctx, http.MethodPost, path, nil, &workspace)
	return &workspace, err
}

// clientDialTimeout bounds reaching the daemon. A daemon on the same
// machine that has not accepted a connection by then is not there.
const clientDialTimeout = 5 * time.Second

// NewClient constructs a Client targeting a daemon listening on port on
// localhost.
//
// Only connecting is bounded, not the whole request. Some answers take as
// long as the work they report on: a scout run scores every job it
// considers, and a local model can take minutes over one cover letter.
// A caller that wants a limit puts it on the context, as Health's callers
// do.
func NewClient(port int) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: clientDialTimeout}).DialContext
	return &Client{
		baseURL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		httpClient: &http.Client{Transport: transport},
	}
}

// Health reports whether the daemon is reachable and responding, and
// returns its version.
func (c *Client) Health(ctx context.Context) (string, error) {
	var resp healthResponse
	_, err := c.do(ctx, http.MethodGet, "/v1/health", nil, &resp)
	return resp.Version, err
}

// InterfaceURL is the address of the interface the daemon serves to a web
// browser, and whether this daemon was built with it (ADR-0035 §1). A
// daemon built without it answers its address with a page saying so, and
// that page's text is returned as the error.
func (c *Client) InterfaceURL(ctx context.Context) (string, error) {
	address := c.baseURL + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach the jobpilot daemon: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		if message := strings.TrimSpace(string(body)); message != "" {
			return "", errors.New(message)
		}
		return "", fmt.Errorf("the daemon answered %s at %s", res.Status, address)
	}
	return address, nil
}

// Shutdown asks the daemon to stop. It answers before it has stopped, so
// the caller waits for its process to go (system.WaitForDaemonExit).
func (c *Client) Shutdown(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodPost, "/v1/shutdown", nil, nil)
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
// discovered first, optionally filtered. It reads local history only and
// contacts no job source, so unlike SearchJobs it works offline. A limit of
// zero means the daemon's default page size.
func (c *Client) ListJobs(ctx context.Context, filter domain.JobFilter, limit, offset int) (*domain.JobPage, error) {
	query := url.Values{}
	if filter.Keywords != "" {
		query.Set("q", filter.Keywords)
	}
	if filter.SourceID != "" {
		query.Set("source", filter.SourceID)
	}
	if filter.RemoteType != "" {
		query.Set("remote", filter.RemoteType)
	}
	if !filter.PostedSince.IsZero() {
		query.Set("posted_since", filter.PostedSince.UTC().Format(time.RFC3339))
	}
	if filter.Interest != "" {
		query.Set("interest", string(filter.Interest))
	}
	if filter.WorkFrom != "" {
		query.Set("work_from", filter.WorkFrom)
	}
	if filter.IncludeRelocation {
		query.Set("include_relocation", "true")
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

// StaleSavedSearches lists the saved searches due to run again.
func (c *Client) StaleSavedSearches(ctx context.Context) ([]*domain.SavedSearch, error) {
	var searches []*domain.SavedSearch
	_, err := c.do(ctx, http.MethodGet, "/v1/searches/stale", nil, &searches)
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

// ListCountries returns the countries a search can work from, and the
// profile's own country when it is one.
func (c *Client) ListCountries(ctx context.Context) (*Countries, error) {
	var countries Countries
	_, err := c.do(ctx, http.MethodGet, "/v1/countries", nil, &countries)
	return &countries, err
}

// ListSearchTemplates returns the catalog of ready-made searches.
func (c *Client) ListSearchTemplates(ctx context.Context) ([]domain.SearchTemplateGroup, error) {
	var groups []domain.SearchTemplateGroup
	_, err := c.do(ctx, http.MethodGet, "/v1/search-templates", nil, &groups)
	return groups, err
}

// ImportSearchTemplates creates a saved search from each named template,
// each starting with workFrom as the country the candidate works from.
func (c *Client) ImportSearchTemplates(ctx context.Context, templateIDs []string, workFrom string) ([]*domain.SavedSearch, error) {
	req := importTemplatesRequest{TemplateIDs: templateIDs, WorkFrom: workFrom}
	var created []*domain.SavedSearch
	_, err := c.do(ctx, http.MethodPost, "/v1/search-templates/import", req, &created)
	return created, err
}

// ListCredentials returns which credentials are configured. It returns
// refs and never values: there is no client method that reads a secret,
// because there is no endpoint that serves one (ADR-0019 §4).
func (c *Client) ListCredentials(ctx context.Context) ([]domain.CredentialRef, error) {
	var refs []domain.CredentialRef
	_, err := c.do(ctx, http.MethodGet, "/v1/credentials", nil, &refs)
	return refs, err
}

// SetCredential stores a secret under a source and label.
func (c *Client) SetCredential(ctx context.Context, source, label, secret string) (*domain.CredentialRef, error) {
	req := setCredentialRequest{Secret: secret}
	var ref domain.CredentialRef
	if _, err := c.do(ctx, http.MethodPut, credentialPath(source, label), req, &ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

// DeleteCredential removes a stored secret.
func (c *Client) DeleteCredential(ctx context.Context, source, label string) error {
	_, err := c.do(ctx, http.MethodDelete, credentialPath(source, label), nil, nil)
	return err
}

func credentialPath(source, label string) string {
	if label == "" {
		label = domain.DefaultCredentialLabel
	}
	return "/v1/credentials/" + url.PathEscape(source) + "/" + url.PathEscape(label)
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
func (c *Client) RunSavedSearch(ctx context.Context, id string) (*domain.DiscoveryResult, error) {
	var result domain.DiscoveryResult
	if _, err := c.do(ctx, http.MethodPost, "/v1/searches/"+url.PathEscape(id)+"/run", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetJob returns one stored job by ID.
func (c *Client) GetJob(ctx context.Context, id string) (*domain.Job, error) {
	var job domain.Job
	if _, err := c.do(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(id), nil, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// ListSources names the sources a search asks, in the order it asks them.
func (c *Client) ListSources(ctx context.Context) ([]domain.SourceInfo, error) {
	var sources []domain.SourceInfo
	_, err := c.do(ctx, http.MethodGet, "/v1/sources", nil, &sources)
	return sources, err
}

// SearchJobs runs a job search against criteria.
func (c *Client) SearchJobs(ctx context.Context, criteria domain.SearchCriteria) (*domain.DiscoveryResult, error) {
	var result domain.DiscoveryResult
	if _, err := c.do(ctx, http.MethodPost, "/v1/jobs/search", criteria, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// GetDiscoverySettings returns how discovery behaves without being asked.
func (c *Client) GetDiscoverySettings(ctx context.Context) (*domain.DiscoverySettings, error) {
	var settings domain.DiscoverySettings
	if _, err := c.do(ctx, http.MethodGet, "/v1/settings/discovery", nil, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// SaveDiscoverySettings saves how discovery behaves without being asked.
func (c *Client) SaveDiscoverySettings(ctx context.Context, settings domain.DiscoverySettings) (*domain.DiscoverySettings, error) {
	var saved domain.DiscoverySettings
	if _, err := c.do(ctx, http.MethodPut, "/v1/settings/discovery", settings, &saved); err != nil {
		return nil, err
	}
	return &saved, nil
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
func (c *Client) CreateApplication(ctx context.Context, jobID, candidateID, resumeID string) (*domain.Application, error) {
	req := createApplicationRequest{JobID: jobID, CandidateID: candidateID, ResumeID: resumeID}
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

// CreateCoverLetter saves a letter the candidate wrote as a new version of
// an application's cover letter, and makes it the current one.
func (c *Client) CreateCoverLetter(ctx context.Context, applicationID, content string) (*domain.CoverLetter, error) {
	var coverLetter domain.CoverLetter
	_, err := c.do(ctx, http.MethodPost, "/v1/applications/"+url.PathEscape(applicationID)+"/cover-letters",
		coverLetterRequest{Content: content}, &coverLetter)
	if err != nil {
		return nil, err
	}
	return &coverLetter, nil
}

// ListCoverLetterVersions returns every version of an application's cover
// letter, oldest first.
func (c *Client) ListCoverLetterVersions(ctx context.Context, applicationID string) ([]*domain.CoverLetter, error) {
	var versions []*domain.CoverLetter
	_, err := c.do(ctx, http.MethodGet, "/v1/applications/"+url.PathEscape(applicationID)+"/cover-letters", nil, &versions)
	return versions, err
}

// EditCoverLetter changes an existing cover letter version in place.
func (c *Client) EditCoverLetter(ctx context.Context, coverLetterID, content string) (*domain.CoverLetter, error) {
	req := coverLetterRequest{Content: content}
	var coverLetter domain.CoverLetter
	_, err := c.do(ctx, http.MethodPut, "/v1/cover-letters/"+url.PathEscape(coverLetterID), req, &coverLetter)
	if err != nil {
		return nil, err
	}
	return &coverLetter, nil
}

// AddQuestion notes a question on an application, unanswered.
func (c *Client) AddQuestion(ctx context.Context, applicationID, label string) (*domain.ApplicationQuestion, error) {
	req := addQuestionRequest{Label: label}
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

// SetQuestionAnswer records the candidate's answer to a question. An empty
// answer clears it.
func (c *Client) SetQuestionAnswer(ctx context.Context, questionID, answer string) (*domain.ApplicationQuestion, error) {
	req := setQuestionAnswerRequest{Answer: answer}
	var question domain.ApplicationQuestion
	_, err := c.do(ctx, http.MethodPut, "/v1/questions/"+url.PathEscape(questionID)+"/answer", req, &question)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// DeleteQuestion removes a question and its answer.
func (c *Client) DeleteQuestion(ctx context.Context, questionID string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/questions/"+url.PathEscape(questionID), nil, nil)
	return err
}

// SetApplyURL records where the application's form is, to open instead of
// the job's own link. An empty link clears it.
func (c *Client) SetApplyURL(ctx context.Context, applicationID, applyURL string) (*domain.Application, error) {
	var app domain.Application
	_, err := c.do(ctx, http.MethodPut, "/v1/applications/"+url.PathEscape(applicationID)+"/apply-url",
		setApplyURLRequest{URL: applyURL}, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
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
	if c.workspaceID != "" && !strings.HasPrefix(path, "/v1/workspaces") {
		req.Header.Set(WorkspaceHeader, c.workspaceID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach jobpilot daemon: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string                `json:"error"`
			Reach *domain.DeletionReach `json:"reach"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errBody); err == nil && errBody.Error != "" {
			if errBody.Reach != nil {
				return resp, &ReachChangedError{Message: errBody.Error, Reach: errBody.Reach}
			}
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

// WakeScout asks the daemon to run a scout if it is due, which is all an
// alarm does. It returns as soon as the daemon has decided.
func (c *Client) WakeScout(ctx context.Context, id string) (*domain.ScoutWake, error) {
	var wake domain.ScoutWake
	if _, err := c.do(ctx, http.MethodPost, "/v1/scouts/"+url.PathEscape(id)+"/wake", nil, &wake); err != nil {
		return nil, err
	}
	return &wake, nil
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

// ListActivity returns one page of agent runs of every kind, newest first.
func (c *Client) ListActivity(ctx context.Context, filter domain.AgentRunFilter) (*domain.AgentRunPage, error) {
	query := url.Values{}
	if filter.TaskType != "" {
		query.Set("task_type", filter.TaskType)
	}
	if filter.Status != "" {
		query.Set("status", string(filter.Status))
	}
	if filter.Limit > 0 {
		query.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		query.Set("offset", strconv.Itoa(filter.Offset))
	}
	path := "/v1/agent-runs"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	var page domain.AgentRunPage
	if _, err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// GetActivityRun returns one agent run with every step it recorded.
func (c *Client) GetActivityRun(ctx context.Context, id string) (*domain.AgentRunDetail, error) {
	var detail domain.AgentRunDetail
	if _, err := c.do(ctx, http.MethodGet, "/v1/agent-runs/"+url.PathEscape(id), nil, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
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
		Schedule:              scout.Schedule,
	}
}

// ReachChangedError is a deletion the daemon refused because what it would
// reach changed since its preview. Reach is what it reaches now, for the
// caller to show and confirm again.
type ReachChangedError struct {
	Message string
	Reach   *domain.DeletionReach
}

func (e *ReachChangedError) Error() string { return e.Message }

// deletionPath is where target lives, as the daemon's deletion routes name it.
func deletionPath(target domain.DeletionTarget, id string) (string, error) {
	for _, route := range deletionRoutes {
		if route.target == target {
			return strings.Replace(route.path, "{id}", url.PathEscape(id), 1), nil
		}
	}
	return "", fmt.Errorf("nothing can be deleted as %q", target)
}

// PreviewDeletion returns what deleting target would take with it, and the
// token Delete needs.
func (c *Client) PreviewDeletion(ctx context.Context, target domain.DeletionTarget, id string) (*domain.DeletionReach, error) {
	path, err := deletionPath(target, id)
	if err != nil {
		return nil, err
	}
	var reach domain.DeletionReach
	if _, err := c.do(ctx, http.MethodGet, path+"/deletion", nil, &reach); err != nil {
		return nil, err
	}
	return &reach, nil
}

// Delete carries out the deletion a preview returned token for. If what it
// reaches changed since, it returns a *ReachChangedError and deletes
// nothing.
func (c *Client) Delete(ctx context.Context, target domain.DeletionTarget, id, token string) (*domain.DeletionResult, error) {
	path, err := deletionPath(target, id)
	if err != nil {
		return nil, err
	}
	var result domain.DeletionResult
	if _, err := c.do(ctx, http.MethodDelete, path+"?"+url.Values{"token": {token}}.Encode(), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetJobInterest rules a job out (notInterested) or back in.
func (c *Client) SetJobInterest(ctx context.Context, id string, notInterested bool) (*domain.Job, error) {
	var job domain.Job
	if _, err := c.do(ctx, http.MethodPut, "/v1/jobs/"+url.PathEscape(id)+"/interest",
		setJobInterestRequest{NotInterested: notInterested}, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// GetAboutYou returns what the candidate has said about themselves.
func (c *Client) GetAboutYou(ctx context.Context) (*services.AboutYou, error) {
	var about services.AboutYou
	if _, err := c.do(ctx, http.MethodGet, "/v1/profile/about", nil, &about); err != nil {
		return nil, err
	}
	return &about, nil
}

// SaveAboutYou changes some of what the candidate has said about
// themselves; an empty answer clears one.
func (c *Client) SaveAboutYou(ctx context.Context, update services.AboutYouUpdate) (*services.AboutYou, error) {
	var about services.AboutYou
	if _, err := c.do(ctx, http.MethodPut, "/v1/profile/about", update, &about); err != nil {
		return nil, err
	}
	return &about, nil
}

// DeleteKeptAnswer forgets a kept answer.
func (c *Client) DeleteKeptAnswer(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/profile/about/kept/"+url.PathEscape(id), nil, nil)
	return err
}

// KeepAnswer keeps the answer the candidate wrote to a question, so a
// similar question later is offered it.
func (c *Client) KeepAnswer(ctx context.Context, questionID string) (*domain.PersonalAnswer, error) {
	var kept domain.PersonalAnswer
	if _, err := c.do(ctx, http.MethodPost, "/v1/questions/"+url.PathEscape(questionID)+"/keep", nil, &kept); err != nil {
		return nil, err
	}
	return &kept, nil
}
