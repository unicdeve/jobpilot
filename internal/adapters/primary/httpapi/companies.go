package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// registerCompanies serves the companies the candidate watches
// (docs/02_features/17_company_watchlists.md, ADR-0024 §8).
func registerCompanies(mux *http.ServeMux, svc *services.CompanyWatchService) {
	mux.HandleFunc("GET /v1/companies", handleListCompanies(svc))
	mux.HandleFunc("POST /v1/companies", handleWatchCompany(svc))
	mux.HandleFunc("GET /v1/companies/suggestions", handleCompanySuggestions(svc))
	mux.HandleFunc("PUT /v1/companies/{id}", handleRenameCompany(svc))
	mux.HandleFunc("DELETE /v1/companies/{id}", handleUnwatchCompany(svc))
	mux.HandleFunc("GET /v1/jobs/{id}/company", handleJobCompany(svc))
}

// companyStatus is the HTTP status for a watch-list error: an address the
// caller should fix is 400, a board its provider says is not there 422, a
// provider that did not answer or asked to be asked later 503, and an
// unknown company 404.
func companyStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrUnsupportedAddress), errors.Is(err, services.ErrEmptyCompanyName),
		errors.Is(err, services.ErrJobNotOnBoard):
		return http.StatusBadRequest
	case errors.Is(err, ports.ErrBoardGone):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ports.ErrSourceUnreachable), errors.Is(err, ports.ErrBoardBusy):
		return http.StatusServiceUnavailable
	case errors.Is(err, ports.ErrWatchedCompanyNotFound):
		return http.StatusNotFound
	}
	return fallbackStatus(err)
}

// Companies is the body of GET /v1/companies.
type Companies struct {
	Companies []*domain.WatchedCompany `json:"companies"`
}

// Suggestions is the body of GET /v1/companies/suggestions.
type Suggestions struct {
	Suggestions []domain.StarterCompany `json:"suggestions"`
}

func handleListCompanies(svc *services.CompanyWatchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companies, err := svc.List(r.Context())
		if err != nil {
			writeError(w, companyStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, Companies{Companies: companies})
	}
}

// watchCompanyRequest names a company by the address of its hiring page or
// of a posting on it, or by a stored job whose posting is on it.
type watchCompanyRequest struct {
	Address string `json:"address,omitempty"`
	JobID   string `json:"job_id,omitempty"`
}

func handleWatchCompany(svc *services.CompanyWatchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req watchCompanyRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if (req.Address == "") == (req.JobID == "") {
			writeError(w, http.StatusBadRequest, errors.New("name either an address or a job_id"))
			return
		}

		var company *domain.WatchedCompany
		var created bool
		var err error
		if req.JobID != "" {
			company, created, err = svc.WatchJob(r.Context(), req.JobID)
		} else {
			company, created, err = svc.Watch(r.Context(), req.Address)
		}
		if err != nil {
			writeError(w, companyStatus(err), err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, company)
	}
}

func handleCompanySuggestions(svc *services.CompanyWatchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		suggestions, err := svc.Suggestions(r.Context())
		if err != nil {
			writeError(w, companyStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, Suggestions{Suggestions: suggestions})
	}
}

type renameCompanyRequest struct {
	Name string `json:"name"`
}

func handleRenameCompany(svc *services.CompanyWatchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req renameCompanyRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		company, err := svc.Rename(r.Context(), r.PathValue("id"), req.Name)
		if err != nil {
			writeError(w, companyStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, company)
	}
}

func handleUnwatchCompany(svc *services.CompanyWatchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Unwatch(r.Context(), r.PathValue("id")); err != nil {
			writeError(w, companyStatus(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleJobCompany(svc *services.CompanyWatchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		company, err := svc.CompanyForJob(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, companyStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, company)
	}
}

// --- client ---

// ListCompanies returns the companies the candidate watches.
func (c *Client) ListCompanies(ctx context.Context) ([]*domain.WatchedCompany, error) {
	var companies Companies
	_, err := c.do(ctx, http.MethodGet, "/v1/companies", nil, &companies)
	return companies.Companies, err
}

// WatchCompany watches the company whose hiring page address is on, and
// reports whether it was not watched already.
func (c *Client) WatchCompany(ctx context.Context, address string) (*domain.WatchedCompany, bool, error) {
	var company domain.WatchedCompany
	resp, err := c.do(ctx, http.MethodPost, "/v1/companies", watchCompanyRequest{Address: address}, &company)
	if err != nil {
		return nil, false, err
	}
	return &company, resp.StatusCode == http.StatusCreated, nil
}

// RenameCompany changes what a watched company is called.
func (c *Client) RenameCompany(ctx context.Context, id, name string) (*domain.WatchedCompany, error) {
	var company domain.WatchedCompany
	if _, err := c.do(ctx, http.MethodPut, "/v1/companies/"+url.PathEscape(id), renameCompanyRequest{Name: name}, &company); err != nil {
		return nil, err
	}
	return &company, nil
}

// UnwatchCompany stops watching a company.
func (c *Client) UnwatchCompany(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v1/companies/"+url.PathEscape(id), nil, nil)
	return err
}

// CompanySuggestions returns the starter list, less what is watched.
func (c *Client) CompanySuggestions(ctx context.Context) ([]domain.StarterCompany, error) {
	var suggestions Suggestions
	if _, err := c.do(ctx, http.MethodGet, "/v1/companies/suggestions", nil, &suggestions); err != nil {
		return nil, err
	}
	return suggestions.Suggestions, nil
}
