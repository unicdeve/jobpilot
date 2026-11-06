package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// --- applications ---

type createApplicationRequest struct {
	JobID       string `json:"job_id"`
	CandidateID string `json:"candidate_id"`
	ResumeID    string `json:"resume_id"`
}

func handleCreateApplication(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createApplicationRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		app, err := svc.CreateApplication(r.Context(), req.JobID, req.CandidateID, req.ResumeID)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, app)
	}
}

func handleListApplications(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		candidateID := r.URL.Query().Get("candidate_id")
		apps, err := svc.ListApplications(r.Context(), candidateID)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, apps)
	}
}

func handleGetApplication(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app, err := svc.GetApplication(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, app)
	}
}

type transitionApplicationRequest struct {
	ToState domain.ApplicationState `json:"to_state"`
	Reason  string                  `json:"reason"`
}

func handleTransitionApplication(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req transitionApplicationRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		app, err := svc.TransitionState(r.Context(), r.PathValue("id"), req.ToState, req.Reason)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, app)
	}
}

func handleApplicationHistory(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		history, err := svc.ListHistory(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, history)
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

// --- cover letters ---

type coverLetterRequest struct {
	Content string `json:"content"`
}

// coverLetterStatus is the status for a cover letter error: an empty
// letter is the client's.
func coverLetterStatus(err error) int {
	if errors.Is(err, services.ErrEmptyCoverLetter) {
		return http.StatusBadRequest
	}
	return fallbackStatus(err)
}

// handleCreateCoverLetter saves a letter the candidate wrote as a new
// version (ADR-0034 §1).
func handleCreateCoverLetter(svc *services.CoverLetterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req coverLetterRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		coverLetter, err := svc.Create(r.Context(), r.PathValue("id"), req.Content)
		if err != nil {
			writeError(w, coverLetterStatus(err), err)
			return
		}
		writeJSON(w, http.StatusCreated, coverLetter)
	}
}

func handleListCoverLetterVersions(svc *services.CoverLetterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := svc.ListVersions(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, versions)
	}
}

func handleEditCoverLetter(svc *services.CoverLetterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req coverLetterRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		coverLetter, err := svc.Edit(r.Context(), r.PathValue("id"), req.Content)
		if err != nil {
			writeError(w, coverLetterStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, coverLetter)
	}
}

// --- application questions ---

type addQuestionRequest struct {
	Label string `json:"label"`
}

func handleAddQuestion(svc *services.AnswerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addQuestionRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		question, err := svc.AddQuestion(r.Context(), r.PathValue("id"), req.Label)
		if errors.Is(err, services.ErrEmptyQuestion) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, question)
	}
}

func handleListQuestions(svc *services.AnswerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		questions, err := svc.ListByApplication(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, questions)
	}
}

type setQuestionAnswerRequest struct {
	Answer string `json:"answer"`
}

func handleSetQuestionAnswer(svc *services.AnswerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setQuestionAnswerRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		question, err := svc.SetAnswer(r.Context(), r.PathValue("id"), req.Answer)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, question)
	}
}

func handleDeleteQuestion(svc *services.AnswerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if writeErrIfAny(w, svc.DeleteQuestion(r.Context(), r.PathValue("id"))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleAllowedTransitions serves the state machine's own answer for one
// application, so a client can offer the actions that will actually work
// rather than every state there is and an error for most of them.
func handleAllowedTransitions(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		application, err := svc.GetApplication(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, services.AllowedTransitions(application))
	}
}
