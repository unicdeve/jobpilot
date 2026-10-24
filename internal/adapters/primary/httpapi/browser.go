package httpapi

import (
	"errors"
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

// handlePrepareInBrowser starts a run and answers with it at once: the run
// belongs to the daemon, and may wait minutes for the user to answer a
// check in the browser window (ADR-0033 §5).
func handlePrepareInBrowser(browserSvc *services.BrowserWorkerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, err := browserSvc.StartPreparation(r.Context(), r.PathValue("id"))
		if errors.Is(err, services.ErrAssistDoesNotFillForms) || errors.Is(err, services.ErrPreparationInProgress) ||
			errors.Is(err, services.ErrCannotPrepare) {
			writeError(w, http.StatusConflict, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusAccepted, run)
	}
}

func handleStopAgentRun(browserSvc *services.BrowserWorkerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := browserSvc.StopPreparation(r.PathValue("id")); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type setApplyURLRequest struct {
	URL string `json:"url"`
}

func handleSetApplyURL(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setApplyURLRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		app, err := svc.SetApplyURL(r.Context(), r.PathValue("id"), req.URL)
		if errors.Is(err, services.ErrInvalidApplyURL) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
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
