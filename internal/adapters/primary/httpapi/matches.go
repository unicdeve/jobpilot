package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/services"
)

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

// handleGetJobMatch serves a job's latest match, or 404 when it has none:
// no analysis yet is an ordinary answer, which the desktop reads as null.
func handleGetJobMatch(svc *services.MatchingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		match, err := svc.GetMatchForJob(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		if match == nil {
			writeError(w, http.StatusNotFound, errors.New("this job has not been analyzed"))
			return
		}
		writeJSON(w, http.StatusOK, match)
	}
}
