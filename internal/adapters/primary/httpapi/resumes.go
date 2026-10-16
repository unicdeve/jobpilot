package httpapi

import (
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/services"
)

type importResumeRequest struct {
	CandidateID string `json:"candidate_id"`
	FilePath    string `json:"file_path"`
}

func handleImportResume(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req importResumeRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		resume, err := svc.ImportResume(r.Context(), req.CandidateID, req.FilePath)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resume)
	}
}

func handleListResumes(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		candidateID := r.URL.Query().Get("candidate_id")
		resumes, err := svc.GetResumesByCandidate(r.Context(), candidateID)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resumes)
	}
}

type proposeDraftRequest struct {
	ResumeMarkdown string `json:"resume_markdown"`
}

func handleProposeDraft(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req proposeDraftRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		draft, err := svc.ProposeProfileDraft(r.Context(), req.ResumeMarkdown)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, draft)
	}
}
