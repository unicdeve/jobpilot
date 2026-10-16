package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

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
	// Schedule is when the scout runs on its own. Left out, it is off.
	Schedule domain.ScoutSchedule `json:"schedule"`
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
		Schedule:              r.Schedule,
	}
}

// scoutStatus is the status for an error saving a scout: a saved search
// that does not exist is 422, a scout that does not exist 404, and a bad
// score, an impossible ceiling or a missing name 400, the caller's to fix.
func scoutStatus(err error) int {
	if errors.Is(err, ports.ErrUnknownReference) || errors.Is(err, ports.ErrNotFound) {
		return fallbackStatus(err)
	}
	return http.StatusBadRequest
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
			writeError(w, scoutStatus(err), err)
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
			writeError(w, scoutStatus(err), err)
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

// handleWakeScout is what a scout's alarm asks, through `jobpilot scout
// wake`: run it if it is due (ADR-0027 §4). It answers at once; a run it
// starts belongs to the daemon, not to this request.
func handleWakeScout(svc *services.ScoutService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wake, err := svc.Wake(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, wake)
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
