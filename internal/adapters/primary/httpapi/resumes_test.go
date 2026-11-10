package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// uploadResume posts a resume as the interface in a browser does: the
// file's bytes and name, with no path (ADR-0035 §4).
func uploadResume(t *testing.T, client *Client, candidateID, fileName string, content []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	require.NoError(t, form.WriteField("candidate_id", candidateID))
	if fileName != "" {
		part, err := form.CreateFormFile("file", fileName)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, form.Close())

	req, err := http.NewRequest(http.MethodPost, client.baseURL+"/v1/resumes/upload", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", form.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestUploadResume(t *testing.T) {
	client, candRepo, _ := testServer(t)
	ctx := context.Background()
	require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1", FirstName: "Ada"}))

	t.Run("the file is converted and kept under its name, with no path", func(t *testing.T) {
		res := uploadResume(t, client, "cand-1", "Ada Lovelace CV.docx", []byte("analytical engine"))
		require.Equal(t, http.StatusOK, res.StatusCode)

		var resume domain.Resume
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resume))
		assert.NotEmpty(t, resume.ID)
		assert.Equal(t, "Ada Lovelace CV.docx", resume.Name)
		assert.Empty(t, resume.OriginalFilePath, "a browser never has the path")
		assert.Contains(t, resume.MarkdownContent, "Converted from Ada Lovelace CV.docx: analytical engine")

		resumes, err := client.ListResumes(ctx, "cand-1")
		require.NoError(t, err)
		require.Len(t, resumes, 1)
	})

	t.Run("a name sent with a path is kept as the name alone", func(t *testing.T) {
		res := uploadResume(t, client, "cand-1", "../../etc/cv.pdf", []byte("x"))
		require.Equal(t, http.StatusOK, res.StatusCode)
		var resume domain.Resume
		require.NoError(t, json.NewDecoder(res.Body).Decode(&resume))
		assert.Equal(t, "cv.pdf", resume.Name)
	})

	t.Run("no file is a bad request", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, uploadResume(t, client, "cand-1", "", nil).StatusCode)
	})

	t.Run("an empty file is a bad request", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, uploadResume(t, client, "cand-1", "cv.pdf", nil).StatusCode)
	})

	t.Run("a candidate that does not exist is named as the problem", func(t *testing.T) {
		res := uploadResume(t, client, "cand-404", "cv.pdf", []byte("x"))
		assert.Equal(t, http.StatusUnprocessableEntity, res.StatusCode)
	})

	t.Run("a file far larger than a resume is refused", func(t *testing.T) {
		res := uploadResume(t, client, "cand-1", "cv.pdf", bytes.Repeat([]byte("a"), maxResumeUpload+1))
		assert.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)
	})
}
