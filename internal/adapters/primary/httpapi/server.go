// Package httpapi is the daemon's local IPC boundary: a JSON API served over
// loopback HTTP, and a client used by other local processes (currently the
// CLI) to reach it. See docs/09_decisions/0012-loopback-http-ipc.md and
// docs/01_architecture/07_ipc.md.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/llm"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// Services bundles the core services the daemon exposes over HTTP. It
// mirrors exactly what cmd/jobpilot-cli wired directly before this package
// existed — this is a transport change, not a behavior change.
type Services struct {
	Candidates  ports.CandidateRepository
	Onboarding  *services.OnboardingService
	Profile     *services.ProfileService
	Resume      *services.ResumeService
	Discovery   *services.JobDiscoveryService
	Searches    *services.SavedSearchService
	Matching    *services.MatchingService
	Settings    *services.SettingsService
	Application *services.ApplicationService
	CoverLetter *services.CoverLetterService
	Answers     *services.AnswerService
	Browser     *services.BrowserWorkerService
	Scouts      *services.ScoutService
	Credentials *services.CredentialService
}

// NewHandler builds the daemon's HTTP handler over svc.
func NewHandler(svc Services) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/health", handleHealth)

	mux.HandleFunc("POST /v1/onboarding/environment", handleSetupEnvironment(svc.Onboarding))
	mux.HandleFunc("POST /v1/onboarding/llm-check", handleConfigureLLM(svc.Onboarding))
	mux.HandleFunc("POST /v1/onboarding/profile", handleInitializeProfile(svc.Onboarding))

	mux.HandleFunc("GET /v1/profile", handleGetProfile(svc.Candidates))
	mux.HandleFunc("PUT /v1/profile", handleUpdateProfile(svc.Profile))

	mux.HandleFunc("POST /v1/resumes", handleImportResume(svc.Resume))
	mux.HandleFunc("GET /v1/resumes", handleListResumes(svc.Resume))
	mux.HandleFunc("DELETE /v1/resumes/{id}", handleDeleteResume(svc.Resume))
	mux.HandleFunc("POST /v1/resumes/propose-draft", handleProposeDraft(svc.Resume))

	mux.HandleFunc("GET /v1/jobs", handleListJobs(svc.Discovery))
	mux.HandleFunc("GET /v1/jobs/{id}", handleGetJob(svc.Discovery))
	mux.HandleFunc("POST /v1/jobs/search", handleSearchJobs(svc.Discovery))
	mux.HandleFunc("POST /v1/jobs/import", handleImportJob(svc.Discovery))

	mux.HandleFunc("GET /v1/searches", handleListSavedSearches(svc.Searches))
	mux.HandleFunc("POST /v1/searches", handleCreateSavedSearch(svc.Searches))
	mux.HandleFunc("GET /v1/searches/{id}", handleGetSavedSearch(svc.Searches))
	mux.HandleFunc("PUT /v1/searches/{id}", handleUpdateSavedSearch(svc.Searches))
	mux.HandleFunc("DELETE /v1/searches/{id}", handleDeleteSavedSearch(svc.Searches))
	mux.HandleFunc("POST /v1/searches/{id}/default", handleSetDefaultSavedSearch(svc.Searches))
	mux.HandleFunc("POST /v1/searches/{id}/run", handleRunSavedSearch(svc.Searches, svc.Discovery))
	mux.HandleFunc("GET /v1/search-templates", handleListSearchTemplates(svc.Searches))
	mux.HandleFunc("POST /v1/search-templates/import", handleImportSearchTemplates(svc.Searches))

	mux.HandleFunc("GET /v1/scouts", handleListScouts(svc.Scouts))
	mux.HandleFunc("POST /v1/scouts", handleCreateScout(svc.Scouts))
	mux.HandleFunc("GET /v1/scouts/{id}", handleGetScout(svc.Scouts))
	mux.HandleFunc("PUT /v1/scouts/{id}", handleUpdateScout(svc.Scouts))
	mux.HandleFunc("DELETE /v1/scouts/{id}", handleDeleteScout(svc.Scouts))
	mux.HandleFunc("POST /v1/scouts/{id}/run", handleRunScout(svc.Scouts))
	mux.HandleFunc("GET /v1/scout-runs", handleListScoutRuns(svc.Scouts))
	mux.HandleFunc("GET /v1/scout-runs/{id}/events", handleListScoutRunEvents(svc.Scouts))

	mux.HandleFunc("POST /v1/matches/analyze", handleAnalyzeMatch(svc.Matching))
	mux.HandleFunc("GET /v1/matches", handleListMatches(svc.Matching))

	// Credentials go in and never come out (ADR-0019 §4). There is
	// deliberately no route that returns a secret: the daemon has no
	// authentication (ADR-0012), so one that did would make a keychain
	// secret readable over loopback by any process running as the user.
	mux.HandleFunc("GET /v1/credentials", handleListCredentials(svc.Credentials))
	mux.HandleFunc("PUT /v1/credentials/{source}/{label}", handleSetCredential(svc.Credentials))
	mux.HandleFunc("DELETE /v1/credentials/{source}/{label}", handleDeleteCredential(svc.Credentials))

	mux.HandleFunc("GET /v1/settings/llm", handleGetLLMSettings(svc.Settings))
	mux.HandleFunc("PUT /v1/settings/llm", handleSaveLLMSettings(svc.Settings))
	mux.HandleFunc("GET /v1/settings/llm/health", handleLLMHealth(svc.Settings))

	mux.HandleFunc("POST /v1/applications", handleCreateApplication(svc.Application))
	mux.HandleFunc("GET /v1/applications", handleListApplications(svc.Application))
	mux.HandleFunc("GET /v1/applications/{id}", handleGetApplication(svc.Application))
	mux.HandleFunc("POST /v1/applications/{id}/transition", handleTransitionApplication(svc.Application))
	mux.HandleFunc("GET /v1/applications/{id}/history", handleApplicationHistory(svc.Application))
	mux.HandleFunc("GET /v1/applications/{id}/transitions", handleAllowedTransitions(svc.Application))

	mux.HandleFunc("POST /v1/applications/{id}/cover-letter", handleGenerateCoverLetter(svc.CoverLetter))
	mux.HandleFunc("GET /v1/applications/{id}/cover-letters", handleListCoverLetterVersions(svc.CoverLetter))
	mux.HandleFunc("PUT /v1/cover-letters/{id}", handleEditCoverLetter(svc.CoverLetter))

	mux.HandleFunc("POST /v1/applications/{id}/questions", handleAddQuestion(svc.Answers))
	mux.HandleFunc("GET /v1/applications/{id}/questions", handleListQuestions(svc.Answers))
	mux.HandleFunc("POST /v1/questions/{id}/resolve", handleResolveQuestion(svc.Answers))
	mux.HandleFunc("PUT /v1/questions/{id}/answer", handleSetQuestionAnswer(svc.Answers))

	mux.HandleFunc("POST /v1/applications/{id}/prepare-in-browser", handlePrepareInBrowser(svc.Browser, svc.Application))
	mux.HandleFunc("GET /v1/applications/{id}/agent-runs", handleListAgentRuns(svc.Browser))

	return withCORS(mux)
}

// desktopOrigins are the origins the Tauri desktop app's webview can load
// from: the Vite dev server in `tauri dev`, and the packaged app's custom
// protocol on each platform. Browsers only send an Origin header for
// cross-origin requests, so plain HTTP clients (the CLI, curl) are
// unaffected either way; this allowlist exists so an arbitrary website
// open in the user's regular browser can't read local candidate data from
// this loopback daemon via CORS while it happens to be running.
var desktopOrigins = map[string]bool{
	"http://localhost:1420":  true,
	"tauri://localhost":      true,
	"http://tauri.localhost": true,
}

// withCORS lets the desktop webview's fetch calls reach this loopback
// server: WKWebView (and Chromium, on other platforms) treats the webview
// origin and 127.0.0.1 as cross-origin and enforces CORS even for local
// requests, so without these headers every fetch from the desktop app
// fails despite the daemon being reachable.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); desktopOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- onboarding ---

func handleSetupEnvironment(svc *services.OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, err := svc.SetupEnvironment(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"data_dir": dir})
	}
}

func handleConfigureLLM(svc *services.OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := svc.ConfigureLLM(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleInitializeProfile(svc *services.OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var profile domain.Candidate
		if err := readJSON(r, &profile); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if err := svc.InitializeProfile(r.Context(), &profile); writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

// --- profile ---

func handleGetProfile(repo ports.CandidateRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, err := repo.GetFirstCandidate(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

func handleUpdateProfile(svc *services.ProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var profile domain.Candidate
		if err := readJSON(r, &profile); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if err := svc.UpdateProfile(r.Context(), &profile); writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

// --- resumes ---

type importResumeRequest struct {
	CandidateID string `json:"candidate_id"`
	FilePath    string `json:"file_path"`
}

func handleImportResume(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importResumeRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		resume, err := svc.ImportResume(r.Context(), req.CandidateID, req.FilePath)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resume)
	}
}

func handleListResumes(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		candidateID := r.URL.Query().Get("candidate_id")
		resumes, err := svc.GetResumesByCandidate(r.Context(), candidateID)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resumes)
	}
}

func handleDeleteResume(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := svc.DeleteResume(r.Context(), id); writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

type proposeDraftRequest struct {
	ResumeMarkdown string `json:"resume_markdown"`
}

func handleProposeDraft(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req proposeDraftRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		draft, err := svc.ProposeProfileDraft(r.Context(), req.ResumeMarkdown)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, draft)
	}
}

// --- jobs ---

// savedSearchRequest is the body for creating or updating a saved search.
type savedSearchRequest struct {
	Name     string                `json:"name"`
	Criteria domain.SearchCriteria `json:"criteria"`
}

func handleListSavedSearches(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		searches, err := svc.List(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, searches)
	}
}

func handleCreateSavedSearch(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req savedSearchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		search, err := svc.Create(r.Context(), req.Name, req.Criteria)
		if err != nil {
			// A rejected name or empty criteria is the caller's mistake to
			// correct, not a server failure.
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, search)
	}
}

func handleGetSavedSearch(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		search, err := svc.Get(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, search)
	}
}

func handleUpdateSavedSearch(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req savedSearchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		search, err := svc.Update(r.Context(), r.PathValue("id"), req.Name, req.Criteria)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, search)
	}
}

func handleDeleteSavedSearch(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if writeErrIfAny(w, svc.Delete(r.Context(), r.PathValue("id"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleSetDefaultSavedSearch(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		search, err := svc.SetDefault(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, search)
	}
}

// handleListSearchTemplates serves the catalog of ready-made searches.
//
// The catalog is static, so this needs no query parameters and returns the
// same thing to every caller: the desktop application and the CLI render one
// list, not two that can drift apart.
func handleListSearchTemplates(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, svc.Templates())
	}
}

// importTemplatesRequest names the templates to adopt, by template ID.
type importTemplatesRequest struct {
	TemplateIDs []string `json:"template_ids"`
}

func handleImportSearchTemplates(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importTemplatesRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		created, err := svc.ImportTemplates(r.Context(), req.TemplateIDs)
		if err != nil {
			// An unknown template ID and an empty selection are both the
			// caller's to correct.
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

// handleRunSavedSearch runs discovery using a saved search's criteria. It
// exists so that neither the CLI nor the desktop has to read a search, hold
// its criteria, and post them back — which is the shape that let an empty
// search be submitted in the first place.
func handleRunSavedSearch(searches *services.SavedSearchService, discovery *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		search, err := searches.Get(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}

		jobs, err := discovery.Search(r.Context(), search.Criteria)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, jobs)
	}
}

// --- credentials ---

// setCredentialRequest carries the secret being stored. It has no response
// counterpart: nothing returns a secret.
type setCredentialRequest struct {
	Secret string `json:"secret"`
}

// handleListCredentials serves which credentials are configured, and no
// values. The response is refs only, which are safe to show and to log.
func handleListCredentials(svc *services.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refs, err := svc.List(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, refs)
	}
}

func handleSetCredential(svc *services.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setCredentialRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		ref, err := svc.Set(r.Context(), r.PathValue("source"), r.PathValue("label"), req.Secret)
		if err != nil {
			// A malformed ref and an empty secret are the caller's to
			// correct; a keychain that never answered is not, and telling
			// the user to fix a correct request would send them looking in
			// the wrong place. The error names the ref and never the
			// secret either way.
			writeError(w, credentialStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, ref)
	}
}

func handleDeleteCredential(svc *services.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Delete(r.Context(), r.PathValue("source"), r.PathValue("label")); err != nil {
			writeError(w, credentialStatus(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// credentialStatus separates a request the caller should fix from a
// keystore that could not be reached.
func credentialStatus(err error) int {
	if errors.Is(err, ports.ErrCredentialStoreUnavailable) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

// handleListJobs serves already-discovered jobs from local storage. It is
// the read counterpart to POST /v1/jobs/search, which contacts external job
// sources and stores what it finds; browsing history must not require a
// network round trip (docs/02_features/04_job_discovery.md AC-006).
func handleListJobs(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		limit, err := intQueryParam(query, "limit")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		offset, err := intQueryParam(query, "offset")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		page, err := svc.ListJobs(r.Context(), query.Get("q"), limit, offset)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}

// handleGetJob serves one stored job, the read behind the desktop app's job
// detail screen.
func handleGetJob(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := svc.GetJob(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func handleSearchJobs(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var criteria domain.SearchCriteria
		if err := readJSON(r, &criteria); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		jobs, err := svc.Search(r.Context(), criteria)
		if errors.Is(err, services.ErrEmptyCriteria) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, jobs)
	}
}

type importJobRequest struct {
	URL string `json:"url"`
}

func handleImportJob(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importJobRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		job, err := svc.ImportJob(r.Context(), req.URL)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

// --- matches ---

type analyzeMatchRequest struct {
	CandidateID string `json:"candidate_id"`
	JobID       string `json:"job_id"`
}

func handleAnalyzeMatch(svc *services.MatchingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req analyzeMatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		match, err := svc.AnalyzeJobForCandidate(r.Context(), req.CandidateID, req.JobID)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, match)
	}
}

func handleListMatches(svc *services.MatchingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		candidateID := r.URL.Query().Get("candidate_id")
		matches, err := svc.ListMatches(r.Context(), candidateID)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, matches)
	}
}

// --- settings ---

func handleGetLLMSettings(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.GetLLMSettings(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

// saveLLMSettingsResponse carries the saved settings back, plus a
// non-fatal connectivity warning: saving must succeed even if the new
// backend can't be reached right now, matching the CLI's prior in-process
// behavior of warning rather than failing in that case.
type saveLLMSettingsResponse struct {
	Settings          domain.LLMSettings `json:"settings"`
	ConnectivityError string             `json:"connectivity_error,omitempty"`
}

func handleSaveLLMSettings(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var settings domain.LLMSettings
		if err := readJSON(r, &settings); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if err := svc.SaveLLMSettings(r.Context(), &settings); writeErrIfAny(w, err) {
			return
		}

		resp := saveLLMSettingsResponse{Settings: settings}
		if provider, err := llm.NewProvider(settings); err != nil {
			resp.ConnectivityError = err.Error()
		} else if err := provider.TestConnectivity(r.Context()); err != nil {
			resp.ConnectivityError = err.Error()
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleAllowedTransitions serves the state machine's own answer for one
// application, so a client can offer the actions that will actually work
// rather than every state there is and an error for most of them.
func handleAllowedTransitions(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		application, err := svc.GetApplication(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, services.AllowedTransitions(application))
	}
}

// llmHealthCheckTimeout bounds the connectivity probe well under the
// providers' own 30s request timeout: the probe runs before a UI action
// starts, so an unreachable host must fail fast rather than leave the user
// waiting on a button.
const llmHealthCheckTimeout = 5 * time.Second

// llmHealthResponse reports whether a generation request would work right
// now. Configured and OK are separate because the two failures need
// different advice: nothing has been set up yet, versus a backend that is
// set up but not answering.
type llmHealthResponse struct {
	Configured bool   `json:"configured"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

// handleLLMHealth lets a caller check the configured backend before starting
// work that depends on it, rather than discovering the problem partway
// through and leaving the user with half a result.
func handleLLMHealth(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.GetLLMSettings(r.Context())
		if writeErrIfAny(w, err) {
			return
		}

		// Unconfigured settings resolve to the Ollama defaults, which may
		// well answer — but the user still has to choose a model that is
		// actually installed, so there is nothing to probe for yet.
		if settings.Provider == "" {
			writeJSON(w, http.StatusOK, llmHealthResponse{})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), llmHealthCheckTimeout)
		defer cancel()

		resp := llmHealthResponse{Configured: true}
		provider, err := llm.NewProvider(*settings)
		if err == nil {
			err = provider.TestConnectivity(ctx)
		}
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.OK = true
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// intQueryParam reads an optional non-negative integer query parameter.
// An absent or empty parameter yields 0, which callers read as "unset".
func intQueryParam(query url.Values, name string) (int, error) {
	raw := query.Get(name)
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be a whole number", name, raw)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s %d: must not be negative", name, value)
	}
	return value, nil
}

// --- JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// writeErrIfAny writes a 500 response and returns true if err is non-nil, so
// handlers can do `if writeErrIfAny(w, err) { return }`.
func writeErrIfAny(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	writeError(w, http.StatusInternalServerError, err)
	return true
}

func readJSON(r *http.Request, dest any) error {
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

// scoutRequest is the body for creating or updating a scout.
//
// It carries no salary floor, company blocklist or location rule: the
// scout's saved search holds those, and SearchCriteria.Matches applies
// them (ADR-0018 §1).
type scoutRequest struct {
	Name                  string             `json:"name"`
	SavedSearchID         string             `json:"saved_search_id"`
	MinMatchScore         int                `json:"min_match_score"`
	MaxApplicationsPerRun int                `json:"max_applications_per_run"`
	Source                domain.ScoutSource `json:"source"`
	IsEnabled             bool               `json:"is_enabled"`
}

func (r scoutRequest) toScout(id string) *domain.Scout {
	return &domain.Scout{
		ID:                    id,
		Name:                  r.Name,
		SavedSearchID:         r.SavedSearchID,
		MinMatchScore:         r.MinMatchScore,
		MaxApplicationsPerRun: r.MaxApplicationsPerRun,
		Source:                r.Source,
		IsEnabled:             r.IsEnabled,
	}
}

func handleListScouts(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scouts, err := svc.ListScouts(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, scouts)
	}
}

func handleCreateScout(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req scoutRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		scout := req.toScout("")
		if err := svc.CreateScout(r.Context(), scout); err != nil {
			// A bad score, an impossible ceiling or a missing search is
			// the caller's to fix.
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, scout)
	}
}

func handleGetScout(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scout, err := svc.GetScout(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, scout)
	}
}

func handleUpdateScout(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req scoutRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		scout := req.toScout(r.PathValue("id"))
		if err := svc.UpdateScout(r.Context(), scout); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, scout)
	}
}

func handleDeleteScout(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteScout(r.Context(), r.PathValue("id")); writeErrIfAny(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRunScout(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// A run that fails partway keeps the applications it created, so
		// the report is returned either way and the error travels inside
		// it (AC-006).
		report, err := svc.Run(r.Context(), r.PathValue("id"))
		if report == nil {
			writeErrIfAny(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

func handleListScoutRuns(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		runs, err := svc.ListRuns(r.Context(), limit)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

func handleListScoutRunEvents(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := svc.ListRunEvents(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, events)
	}
}
