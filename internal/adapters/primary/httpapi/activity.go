package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// --- agent activity ---

// handleListActivity serves one page of every agent run, newest first,
// optionally narrowed by task_type and status.
func handleListActivity(svc *services.AgentActivityService) http.HandlerFunc {
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

		page, err := svc.ListRuns(r.Context(), domain.AgentRunFilter{
			TaskType: query.Get("task_type"),
			Status:   domain.AgentRunStatus(query.Get("status")),
			Limit:    limit,
			Offset:   offset,
		})
		if errors.Is(err, services.ErrInvalidActivityFilter) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}

// handleGetActivityRun serves one run with every step it recorded.
func handleGetActivityRun(svc *services.AgentActivityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, err := svc.GetRun(r.Context(), r.PathValue("id"))
		if errors.Is(err, ports.ErrAgentRunNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, detail)
	}
}
