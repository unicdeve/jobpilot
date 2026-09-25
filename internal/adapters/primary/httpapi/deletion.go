package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// deletionRoutes are the things that can be deleted, at the path of what
// each deletes (docs/02_features/15_data_deletion.md). Each has a
// read-only preview at <path>/deletion, and a DELETE that carries out the
// preview the user confirmed, named by its token (ADR-0021 §4).
var deletionRoutes = []struct {
	path   string
	target domain.DeletionTarget
}{
	{"/v1/jobs/{id}", domain.DeleteJob},
	{"/v1/applications/{id}", domain.DeleteApplication},
	{"/v1/cover-letters/{id}", domain.DeleteCoverLetter},
	{"/v1/resumes/{id}", domain.DeleteResume},
	{"/v1/profile", domain.DeleteProfile},
	{"/v1/data/activity", domain.ClearActivity},
	{"/v1/data/jobs", domain.ClearJobs},
	{"/v1/data", domain.DeleteEverything},
}

func registerDeletions(mux *http.ServeMux, svc *services.DeletionService) {
	for _, route := range deletionRoutes {
		mux.HandleFunc("GET "+route.path+"/deletion", handlePreviewDeletion(svc, route.target))
		mux.HandleFunc("DELETE "+route.path, handleDelete(svc, route.target))
	}
}

func handlePreviewDeletion(svc *services.DeletionService, target domain.DeletionTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reach, err := svc.Preview(r.Context(), target, r.PathValue("id"))
		if err != nil {
			writeDeletionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, reach)
	}
}

func handleDelete(svc *services.DeletionService, target domain.DeletionTarget) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := svc.Delete(r.Context(), target, r.PathValue("id"), r.URL.Query().Get("token"))
		if err != nil {
			writeDeletionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// deletionConflict is a 409 for a deletion whose reach changed since it was
// confirmed. It carries the reach as it is now, so the client can show it
// and ask again rather than make a second request to find out.
type deletionConflict struct {
	Error string                `json:"error"`
	Reach *domain.DeletionReach `json:"reach"`
}

func writeDeletionError(w http.ResponseWriter, err error) {
	var changed *services.ReachChangedError
	switch {
	case errors.As(err, &changed):
		writeJSON(w, http.StatusConflict, deletionConflict{Error: err.Error(), Reach: changed.Reach})
	case errors.Is(err, services.ErrInvalidDeletion), errors.Is(err, services.ErrDeletionUnconfirmed):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, services.ErrNothingToDelete):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, services.ErrDeletionRefused):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

type setJobInterestRequest struct {
	NotInterested bool `json:"not_interested"`
}

// handleSetJobInterest rules a job out or back in. It is not a deletion,
// and is reversible, so it takes no token.
func handleSetJobInterest(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setJobInterestRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		job, err := svc.SetInterest(r.Context(), r.PathValue("id"), req.NotInterested)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}
