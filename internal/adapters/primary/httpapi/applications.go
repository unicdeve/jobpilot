package httpapi

import (
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// --- applications ---

type createApplicationRequest struct {
	JobID       string                 `json:"job_id"`
	CandidateID string                 `json:"candidate_id"`
	ResumeID    string                 `json:"resume_id"`
	Mode        domain.ApplicationMode `json:"mode"`
}

func handleCreateApplication(svc *services.ApplicationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createApplicationRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		app, err := svc.CreateApplication(r.Context(), req.JobID, req.CandidateID, req.ResumeID, req.Mode)
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

// --- cover letters ---

func handleGenerateCoverLetter(svc *services.CoverLetterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		coverLetter, err := svc.Generate(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, coverLetter)
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

type editCoverLetterRequest struct {
	Content string `json:"content"`
}

func handleEditCoverLetter(svc *services.CoverLetterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req editCoverLetterRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		coverLetter, err := svc.Edit(r.Context(), r.PathValue("id"), req.Content)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, coverLetter)
	}
}

// --- application questions ---

type addQuestionRequest struct {
	Label    string              `json:"label"`
	Type     domain.QuestionType `json:"type"`
	Required bool                `json:"required"`
}

func handleAddQuestion(svc *services.AnswerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addQuestionRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		question, err := svc.AddQuestion(r.Context(), r.PathValue("id"), req.Label, req.Type, req.Required)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, question)
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

func handleResolveQuestion(svc *services.AnswerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		question, err := svc.Resolve(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, question)
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

		question, err := svc.SetUserAnswer(r.Context(), r.PathValue("id"), req.Answer)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, question)
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
