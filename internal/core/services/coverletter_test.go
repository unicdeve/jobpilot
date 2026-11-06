package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func newTestCoverLetterService(t *testing.T) (*CoverLetterService, *fakeCoverLetterRepository, *fakeApplicationRepository, *fakeCandidateRepository, *fakeResumeRepository) {
	t.Helper()
	clRepo := newFakeCoverLetterRepository()
	appRepo := newFakeApplicationRepository()
	candRepo := newFakeCandidateRepository()
	resumeRepo := newFakeResumeRepository()
	return NewCoverLetterService(clRepo, appRepo, candRepo, resumeRepo, nil), clRepo, appRepo, candRepo, resumeRepo
}

func TestCoverLetterService_Create(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("saves the candidate's letter as a new version and makes it current", func(t *testing.T) {
		t.Parallel()
		svc, clRepo, appRepo, candRepo, _ := newTestCoverLetterService(t)
		require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))
		require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", CandidateID: "cand-1"}))

		cl, err := svc.Create(ctx, "app-1", "  Dear Acme,\r\nI built your favourite tool.\n  ")

		require.NoError(t, err)
		assert.Equal(t, "Dear Acme,\nI built your favourite tool.", cl.Content, "line endings normalized and the ends trimmed")
		assert.Equal(t, cl.ID, appRepo.applications["app-1"].CurrentCoverLetterID)
		assert.Same(t, cl, clRepo.letters[cl.ID])
	})

	t.Run("a second letter is a new version, and the first is kept", func(t *testing.T) {
		t.Parallel()
		svc, clRepo, appRepo, candRepo, _ := newTestCoverLetterService(t)
		require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))
		require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", CandidateID: "cand-1"}))

		first, err := svc.Create(ctx, "app-1", "version one")
		require.NoError(t, err)
		second, err := svc.Create(ctx, "app-1", "version two")
		require.NoError(t, err)

		assert.NotEqual(t, first.ID, second.ID)
		assert.Len(t, clRepo.letters, 2, "the first version must still exist")
		assert.Equal(t, second.ID, appRepo.applications["app-1"].CurrentCoverLetterID)
	})

	t.Run("an empty letter is refused and nothing is saved", func(t *testing.T) {
		t.Parallel()
		svc, clRepo, appRepo, candRepo, _ := newTestCoverLetterService(t)
		require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))
		require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", CandidateID: "cand-1"}))

		_, err := svc.Create(ctx, "app-1", " \n ")

		assert.ErrorIs(t, err, ErrEmptyCoverLetter)
		assert.Empty(t, clRepo.letters)
	})

	t.Run("an application that does not exist is named", func(t *testing.T) {
		t.Parallel()
		svc, _, _, _, _ := newTestCoverLetterService(t)

		_, err := svc.Create(ctx, "missing-app", "Dear Acme")

		assert.ErrorContains(t, err, "failed to fetch application")
	})
}

func TestCoverLetterService_Edit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, clRepo, appRepo, candRepo, _ := newTestCoverLetterService(t)
	require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))
	require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", CandidateID: "cand-1"}))
	require.NoError(t, clRepo.Save(ctx, &domain.CoverLetter{ID: "cl-1", ApplicationID: "app-1", Content: "original"}))

	edited, err := svc.Edit(ctx, "cl-1", "edited by user")

	require.NoError(t, err)
	assert.Equal(t, "edited by user", edited.Content)
	assert.Equal(t, "edited by user", clRepo.letters["cl-1"].Content)

	_, err = svc.Edit(ctx, "cl-1", "")
	assert.ErrorIs(t, err, ErrEmptyCoverLetter)
	assert.Equal(t, "edited by user", clRepo.letters["cl-1"].Content, "an empty edit changes nothing")
}

func TestCoverLetterService_ListVersions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, clRepo, appRepo, candRepo, _ := newTestCoverLetterService(t)
	require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))
	require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", CandidateID: "cand-1"}))
	require.NoError(t, clRepo.Save(ctx, &domain.CoverLetter{ApplicationID: "app-1"}))
	require.NoError(t, clRepo.Save(ctx, &domain.CoverLetter{ApplicationID: "app-2"}))

	versions, err := svc.ListVersions(ctx, "app-1")

	require.NoError(t, err)
	require.Len(t, versions, 1)
}

// Every way a letter is read carries the flags of its text as it is then,
// checked against the candidate's own material, the resume included
// (docs/02_features/19_draft_checks.md AC-007).
func TestCoverLetterService_Flags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _, appRepo, candRepo, resumeRepo := newTestCoverLetterService(t)
	require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1", Skills: []domain.CandidateSkill{{Name: "Kubernetes"}}}))
	require.NoError(t, resumeRepo.Save(ctx, &domain.Resume{ID: "resume-1", CandidateID: "cand-1", MarkdownContent: "- Ran Kubernetes for 3 years at Acme"}))
	require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", CandidateID: "cand-1", ResumeID: "resume-1"}))

	written, err := svc.Create(ctx, "app-1", "I am excited to apply. I have run Kubernetes for 3 years. I bring 8 years of experience.")
	require.NoError(t, err)

	kinds := func(flags []domain.DraftFlag) []string {
		var out []string
		for _, flag := range flags {
			out = append(out, string(flag.Kind)+": "+flag.Excerpt)
		}
		return out
	}
	// The Kubernetes claim is the resume's; eight years is no one's.
	want := []string{"stock_phrase: I am excited to apply", "unsupported_claim: 8 years"}
	assert.Equal(t, want, kinds(written.Flags))

	versions, err := svc.ListVersions(ctx, "app-1")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, want, kinds(versions[0].Flags))

	edited, err := svc.Edit(ctx, written.ID, "I have run Kubernetes for 3 years.")
	require.NoError(t, err)
	assert.Empty(t, edited.Flags, "an edit is checked as it now reads")
}
