// Package httpapi is the daemon's local IPC boundary: a JSON API served over
// loopback HTTP, and a client used by other local processes (currently the
// CLI) to reach it. See docs/09_decisions/0012-loopback-http-ipc.md and
// docs/01_architecture/07_ipc.md.
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

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
	Matching    *services.MatchingService
	Settings    *services.SettingsService
	Application *services.ApplicationService
	CoverLetter *services.CoverLetterService
	Answers     *services.AnswerService
	Browser     *services.BrowserWorkerService
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
	mux.HandleFunc("POST /v1/jobs/search", handleSearchJobs(svc.Discovery))
	mux.HandleFunc("POST /v1/jobs/import", handleImportJob(svc.Discovery))

	mux.HandleFunc("POST /v1/matches/analyze", handleAnalyzeMatch(svc.Matching))
	mux.HandleFunc("GET /v1/matches", handleListMatches(svc.Matching))

	mux.HandleFunc("GET /v1/settings/llm", handleGetLLMSettings(svc.Settings))
	mux.HandleFunc("PUT /v1/settings/llm", handleSaveLLMSettings(svc.Settings))

	mux.HandleFunc("POST /v1/applications", handleCreateApplication(svc.Application))
	mux.HandleFunc("GET /v1/applications", handleListApplications(svc.Application))
	mux.HandleFunc("GET /v1/applications/{id}", handleGetApplication(svc.Application))
	mux.HandleFunc("POST /v1/applications/{id}/transition", handleTransitionApplication(svc.Application))
	mux.HandleFunc("GET /v1/applications/{id}/history", handleApplicationHistory(svc.Application))

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

func handleSearchJobs(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var criteria domain.SearchCriteria
		if err := readJSON(r, &criteria); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		jobs, err := svc.Search(r.Context(), criteria)
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
