package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

func handleInitializeProfile(svc *services.OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var profile domain.Candidate
		if err := readJSON(r, &profile); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		err := svc.InitializeProfile(r.Context(), &profile)
		if errors.Is(err, services.ErrInvalidTimeZone) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

func handleGetProfile(repo ports.CandidateRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, err := repo.GetFirstCandidate(r.Context())
		// No profile yet is a new installation, not a server failure.
		if errors.Is(err, ports.ErrNoCandidate) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}

func handleUpdateProfile(svc *services.ProfileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var profile domain.Candidate
		if err := readJSON(r, &profile); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		err := svc.UpdateProfile(r.Context(), &profile)
		if errors.Is(err, services.ErrInvalidTimeZone) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, profile)
	}
}
