package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

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

		filter := domain.JobFilter{
			Keywords:   query.Get("q"),
			SourceID:   query.Get("source"),
			RemoteType: query.Get("remote"),
			Interest:   domain.JobInterest(query.Get("interest")),
			WorkFrom:   query.Get("work_from"),
		}
		if raw := query.Get("include_relocation"); raw != "" {
			filter.IncludeRelocation, err = strconv.ParseBool(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("invalid include_relocation %q: must be true or false", raw))
				return
			}
		}
		if raw := query.Get("posted_since"); raw != "" {
			filter.PostedSince, err = time.Parse(time.RFC3339, raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("invalid posted_since %q: must be an RFC 3339 time", raw))
				return
			}
		}

		page, err := svc.ListJobs(r.Context(), filter, limit, offset)
		if errors.Is(err, services.ErrInvalidJobFilter) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
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

// handleListSources names the sources a search asks, so a client can say
// what a search is waiting on while it runs.
func handleListSources(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, svc.SearchSources())
	}
}

func handleSearchJobs(svc *services.JobDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var criteria domain.SearchCriteria
		if err := readJSON(r, &criteria); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		result, err := svc.Search(r.Context(), criteria)
		if err != nil {
			writeError(w, searchStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, result)
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
