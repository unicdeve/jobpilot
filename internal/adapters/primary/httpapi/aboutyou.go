package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// registerAboutYou serves what the candidate says about themselves
// (docs/02_features/16_about_you.md).
func registerAboutYou(mux *http.ServeMux, svc *services.AboutYouService) {
	mux.HandleFunc("GET /v1/profile/about", handleGetAboutYou(svc))
	mux.HandleFunc("PUT /v1/profile/about", handleSaveAboutYou(svc))
	mux.HandleFunc("DELETE /v1/profile/about/kept/{id}", handleDeleteKeptAnswer(svc))
	mux.HandleFunc("POST /v1/questions/{id}/keep", handleKeepAnswer(svc))
	mux.HandleFunc("GET /v1/applications/{id}/personal-questions", handleApplicationQuestions(svc))
	mux.HandleFunc("POST /v1/applications/{id}/personal-questions/prepare", handlePrepareApplicationQuestions(svc))
	mux.HandleFunc("PUT /v1/applications/{id}/personal-questions", handleSaveApplicationAnswers(svc))
}

// aboutYouStatus is the HTTP status for an About you error: no profile is
// a 404, as it is for the profile itself, and a request naming a question
// that does not exist, or an answer that cannot be kept, is the client's.
func aboutYouStatus(err error) int {
	switch {
	case errors.Is(err, ports.ErrNoCandidate):
		return http.StatusNotFound
	case errors.Is(err, services.ErrUnknownPersonalAnswer), errors.Is(err, services.ErrNotKeepable):
		return http.StatusBadRequest
	default:
		return fallbackStatus(err)
	}
}

func handleGetAboutYou(svc *services.AboutYouService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		about, err := svc.Get(r.Context())
		if err != nil {
			writeError(w, aboutYouStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, about)
	}
}

func handleSaveAboutYou(svc *services.AboutYouService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var update services.AboutYouUpdate
		if err := readJSON(r, &update); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		about, err := svc.Save(r.Context(), update)
		if err != nil {
			writeError(w, aboutYouStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, about)
	}
}

func handleDeleteKeptAnswer(svc *services.AboutYouService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteKept(r.Context(), r.PathValue("id")); err != nil {
			writeError(w, aboutYouStatus(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleKeepAnswer(svc *services.AboutYouService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kept, err := svc.Keep(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, aboutYouStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, kept)
	}
}

func handleApplicationQuestions(svc *services.AboutYouService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		questions, err := svc.ApplicationQuestions(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, aboutYouStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, questions)
	}
}

// handlePrepareApplicationQuestions is a POST because it may ask the model
// to propose questions, and stores what it proposes.
func handlePrepareApplicationQuestions(svc *services.AboutYouService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		questions, err := svc.PrepareApplicationQuestions(r.Context(), r.PathValue("id"))
		if err != nil {
			writeError(w, aboutYouStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, questions)
	}
}

type applicationAnswersRequest struct {
	Answers []services.ApplicationAnswer `json:"answers"`
}

func handleSaveApplicationAnswers(svc *services.AboutYouService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req applicationAnswersRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		questions, err := svc.SaveApplicationAnswers(r.Context(), r.PathValue("id"), req.Answers)
		if err != nil {
			writeError(w, aboutYouStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, questions)
	}
}
