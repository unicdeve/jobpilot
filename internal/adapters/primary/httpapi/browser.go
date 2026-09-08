package httpapi

import (
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// --- browser automation ---

// AgentRunWithEvents pairs an AgentRun with its events in one response, so
// a UI activity panel needs only one round trip.
type AgentRunWithEvents struct {
	Run    *domain.AgentRun     `json:"run"`
	Events []*domain.AgentEvent `json:"events"`
}

func handlePrepareInBrowser(browserSvc *services.BrowserWorkerService, appSvc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if err := browserSvc.PrepareApplication(r.Context(), id); writeErrIfAny(w, err) {
			return
		}

		app, err := appSvc.GetApplication(r.Context(), id)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, app)
	}
}

func handleListAgentRuns(svc *services.BrowserWorkerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs, err := svc.ListRuns(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}

		result := make([]AgentRunWithEvents, 0, len(runs))
		for _, run := range runs {
			events, err := svc.ListEvents(r.Context(), run.ID)
			if writeErrIfAny(w, err) {
				return
			}
			result = append(result, AgentRunWithEvents{Run: run, Events: events})
		}

		writeJSON(w, http.StatusOK, result)
	}
}
