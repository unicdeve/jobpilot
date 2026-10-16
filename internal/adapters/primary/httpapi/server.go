// Package httpapi is the daemon's local IPC boundary: a JSON API served over
// loopback HTTP, and a client used by other local processes (currently the
// CLI) to reach it. See docs/09_decisions/0012-loopback-http-ipc.md and
// docs/01_architecture/07_ipc.md.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
	"github.com/unicdev/jobpilot/internal/version"
)

// Services bundles the core services the daemon exposes over HTTP. It
// mirrors exactly what cmd/jobpilot-cli wired directly before this package
// existed — this is a transport change, not a behavior change.
type Services struct {
	Candidates ports.CandidateRepository
	Onboarding *services.OnboardingService
	Profile    *services.ProfileService
	Resume     *services.ResumeService
	Discovery  *services.JobDiscoveryService
	Searches   *services.SavedSearchService
	Matching   *services.MatchingService
	Settings   *services.SettingsService
	// LLM is the configured model, resolved from the stored settings and
	// the keychain on every call, so checking it here checks exactly what
	// a generation would use (ADR-0030 §2).
	LLM           ports.LLMProvider
	Application   *services.ApplicationService
	CoverLetter   *services.CoverLetterService
	Answers       *services.AnswerService
	Browser       *services.BrowserWorkerService
	Scouts        *services.ScoutService
	Credentials   *services.CredentialService
	Activity      *services.AgentActivityService
	Deletion      *services.DeletionService
	AboutYou      *services.AboutYouService
	Companies     *services.CompanyWatchService
	Notifications *services.NotificationService
	// Stop asks the daemon to shut down as it does on SIGTERM, for a
	// newer client replacing it (ADR-0029 §4).
	Stop func()
}

// NewHandler builds the daemon's HTTP handler over svc.
func NewHandler(svc Services) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/health", handleHealth)
	mux.HandleFunc("POST /v1/shutdown", handleShutdown(svc.Stop))

	mux.HandleFunc("POST /v1/onboarding/environment", handleSetupEnvironment(svc.Onboarding))
	mux.HandleFunc("POST /v1/onboarding/llm-check", handleConfigureLLM(svc.Onboarding))
	mux.HandleFunc("POST /v1/onboarding/profile", handleInitializeProfile(svc.Onboarding))

	mux.HandleFunc("GET /v1/profile", handleGetProfile(svc.Candidates))
	mux.HandleFunc("PUT /v1/profile", handleUpdateProfile(svc.Profile))

	mux.HandleFunc("POST /v1/resumes", handleImportResume(svc.Resume))
	mux.HandleFunc("GET /v1/resumes", handleListResumes(svc.Resume))
	mux.HandleFunc("POST /v1/resumes/propose-draft", handleProposeDraft(svc.Resume))

	mux.HandleFunc("GET /v1/jobs", handleListJobs(svc.Discovery))
	mux.HandleFunc("GET /v1/jobs/{id}", handleGetJob(svc.Discovery))
	mux.HandleFunc("GET /v1/sources", handleListSources(svc.Discovery))
	mux.HandleFunc("POST /v1/jobs/search", handleSearchJobs(svc.Discovery))
	mux.HandleFunc("POST /v1/jobs/import", handleImportJob(svc.Discovery))
	mux.HandleFunc("PUT /v1/jobs/{id}/interest", handleSetJobInterest(svc.Discovery))

	mux.HandleFunc("GET /v1/searches", handleListSavedSearches(svc.Searches))
	mux.HandleFunc("POST /v1/searches", handleCreateSavedSearch(svc.Searches))
	mux.HandleFunc("GET /v1/searches/stale", handleStaleSavedSearches(svc.Searches))
	mux.HandleFunc("GET /v1/searches/{id}", handleGetSavedSearch(svc.Searches))
	mux.HandleFunc("PUT /v1/searches/{id}", handleUpdateSavedSearch(svc.Searches))
	mux.HandleFunc("DELETE /v1/searches/{id}", handleDeleteSavedSearch(svc.Searches))
	mux.HandleFunc("POST /v1/searches/{id}/default", handleSetDefaultSavedSearch(svc.Searches))
	mux.HandleFunc("POST /v1/searches/{id}/run", handleRunSavedSearch(svc.Searches, svc.Discovery))
	mux.HandleFunc("GET /v1/search-templates", handleListSearchTemplates(svc.Searches))
	mux.HandleFunc("POST /v1/search-templates/import", handleImportSearchTemplates(svc.Searches))
	mux.HandleFunc("GET /v1/countries", handleListCountries(svc.Candidates))

	mux.HandleFunc("GET /v1/scouts", handleListScouts(svc.Scouts))
	mux.HandleFunc("POST /v1/scouts", handleCreateScout(svc.Scouts))
	mux.HandleFunc("GET /v1/scouts/{id}", handleGetScout(svc.Scouts))
	mux.HandleFunc("PUT /v1/scouts/{id}", handleUpdateScout(svc.Scouts))
	mux.HandleFunc("DELETE /v1/scouts/{id}", handleDeleteScout(svc.Scouts))
	mux.HandleFunc("POST /v1/scouts/{id}/run", handleRunScout(svc.Scouts))
	mux.HandleFunc("POST /v1/scouts/{id}/wake", handleWakeScout(svc.Scouts))
	mux.HandleFunc("GET /v1/scout-runs", handleListScoutRuns(svc.Scouts))
	mux.HandleFunc("GET /v1/scout-runs/{id}/events", handleListScoutRunEvents(svc.Scouts))

	mux.HandleFunc("GET /v1/agent-runs", handleListActivity(svc.Activity))
	mux.HandleFunc("GET /v1/agent-runs/{id}", handleGetActivityRun(svc.Activity))

	mux.HandleFunc("POST /v1/matches/analyze", handleAnalyzeMatch(svc.Matching))
	mux.HandleFunc("GET /v1/matches", handleListMatches(svc.Matching))
	mux.HandleFunc("GET /v1/jobs/{id}/match", handleGetJobMatch(svc.Matching))

	// Credentials go in and never come out (ADR-0019 §4). There is
	// deliberately no route that returns a secret: the daemon has no
	// authentication (ADR-0012), so one that did would make a keychain
	// secret readable over loopback by any process running as the user.
	mux.HandleFunc("GET /v1/credentials", handleListCredentials(svc.Credentials))
	mux.HandleFunc("PUT /v1/credentials/{source}/{label}", handleSetCredential(svc.Credentials))
	mux.HandleFunc("DELETE /v1/credentials/{source}/{label}", handleDeleteCredential(svc.Credentials))

	mux.HandleFunc("GET /v1/settings/llm", handleGetLLMSettings(svc.Settings))
	mux.HandleFunc("PUT /v1/settings/llm", handleSaveLLMSettings(svc))
	mux.HandleFunc("GET /v1/settings/llm/health", handleLLMHealth(svc))
	mux.HandleFunc("GET /v1/settings/discovery", handleGetDiscoverySettings(svc.Settings))
	mux.HandleFunc("PUT /v1/settings/discovery", handleSaveDiscoverySettings(svc.Settings))

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

	registerDeletions(mux, svc.Deletion)
	registerAboutYou(mux, svc.AboutYou)
	registerCompanies(mux, svc.Companies)
	registerNotifications(mux, svc.Notifications)
	registerPrompts(mux, svc.Settings)

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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-JobPilot-Workspace")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// healthResponse says the daemon is up, and which version it is, so a
// client can tell whether to replace it (ADR-0029 §4).
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Version: version.Version})
}

// handleShutdown answers before stopping, so the client hears that it was
// asked. Stopping goes on after the server has closed, while a Scout run
// records how it ended and a notification is delivered, so the client
// waits for the process to go rather than for this answer. It exposes nothing the API does not already: anything that can
// reach it can delete all data (ADR-0012).
func handleShutdown(stop func()) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if stop == nil {
			writeError(w, http.StatusNotImplemented, errors.New("this daemon cannot be stopped over its API"))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		stop()
	}
}
