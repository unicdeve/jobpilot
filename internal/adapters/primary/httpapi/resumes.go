package httpapi

import (
	"errors"
	"fmt"
	"io"
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

// maxResumeUpload is the largest resume POST /v1/resumes/upload accepts.
// A resume is a few pages; anything near this is not one.
const maxResumeUpload = 20 << 20

// handleUploadResume imports a resume sent as its content, as
// multipart/form-data with candidate_id and the file under "file", for the
// interface in a browser, which has no path to send (ADR-0035 §4).
func handleUploadResume(svc *services.ResumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The form's other fields and the multipart framing fit easily in
		// the slack above the file's own limit.
		r.Body = http.MaxBytesReader(w, r.Body, maxResumeUpload+1<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, errResumeTooLarge)
				return
			}
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid resume upload: %w", err))
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid resume upload: no file was sent"))
			return
		}
		defer func() { _ = file.Close() }()
		if header.Size > maxResumeUpload {
			writeError(w, http.StatusRequestEntityTooLarge, errResumeTooLarge)
			return
		}
		data, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("invalid resume upload: %w", err))
			return
		}
		if len(data) == 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("%s is empty", header.Filename))
			return
		}

		resume, err := svc.ImportUploadedResume(r.Context(), r.FormValue("candidate_id"), header.Filename, data)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, resume)
	}
}

var errResumeTooLarge = fmt.Errorf("the resume is larger than %d MB, which is more than any resume needs", maxResumeUpload>>20)

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
