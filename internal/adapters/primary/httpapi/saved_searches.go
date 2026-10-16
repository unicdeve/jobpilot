package httpapi

import (
	"errors"
	"log"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/domain/geo"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

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

// handleStaleSavedSearches lists the searches due to run again, which the
// desktop runs when it opens.
func handleStaleSavedSearches(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		searches, err := svc.Stale(r.Context())
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

// Country is one country a search can work from.
type Country struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Countries is every country, by name, and the profile's own country as a
// code — empty when there is no profile, or its country is not one JobPilot
// can place. A new saved search starts with it (ADR-0023 §4). It also
// carries every time zone, with its country and standard offset, so a
// client can offer a country's zones (ADR-0025 §2).
type Countries struct {
	Countries      []Country      `json:"countries"`
	ProfileCountry string         `json:"profile_country"`
	TimeZones      []geo.TimeZone `json:"time_zones"`
}

// handleListCountries serves the country tables, so no client holds a
// copy that could disagree with the daemon's.
func handleListCountries(candidates ports.CandidateRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := geo.Countries()
		result := Countries{Countries: make([]Country, len(all)), TimeZones: geo.TimeZones()}
		for i, country := range all {
			result.Countries[i] = Country{Code: country.Code, Name: country.Name}
		}
		candidate, err := candidates.GetFirstCandidate(r.Context())
		switch {
		case err == nil && candidate != nil:
			result.ProfileCountry, _ = geo.ResolveCountry(candidate.LocationCountry)
		case err != nil && !errors.Is(err, ports.ErrNoCandidate):
			writeErrIfAny(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// importTemplatesRequest names the templates to adopt, by template ID.
type importTemplatesRequest struct {
	TemplateIDs []string `json:"template_ids"`
	// WorkFrom is the country every created search starts with, if any.
	WorkFrom string `json:"work_from,omitempty"`
}

func handleImportSearchTemplates(svc *services.SavedSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importTemplatesRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		created, err := svc.ImportTemplates(r.Context(), req.TemplateIDs, req.WorkFrom)
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

		result, err := discovery.Search(r.Context(), search.Criteria)
		if err != nil {
			writeError(w, searchStatus(err), err)
			return
		}
		// Discovery answered, so the search has run. Failing to record that
		// costs one repeated search later, not this result.
		if err := searches.RecordRun(r.Context(), search.ID); err != nil {
			log.Printf("recording the run of saved search %s: %v", search.ID, err)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// searchStatus is the HTTP status for a discovery error. No source reached
// is the daemon's upstream being unavailable, and every source failing
// having been reached is a bad gateway, so neither reads as JobPilot
// itself failing.
func searchStatus(err error) int {
	switch {
	case errors.Is(err, services.ErrEmptyCriteria), errors.Is(err, services.ErrInvalidWorkFrom):
		return http.StatusBadRequest
	case errors.Is(err, services.ErrSourcesUnreachable):
		return http.StatusServiceUnavailable
	case errors.Is(err, services.ErrAllSourcesFailed):
		return http.StatusBadGateway
	default:
		return fallbackStatus(err)
	}
}
