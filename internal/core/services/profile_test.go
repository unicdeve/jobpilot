package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestProfileService_GetProfile(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		repo := newFakeCandidateRepository()
		want := &domain.Candidate{ID: "cand-1", Name: "Taiwo"}
		require.NoError(t, repo.Save(context.Background(), want))

		svc := NewProfileService(repo)
		got, err := svc.GetProfile(context.Background(), "cand-1")

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("repo error is wrapped", func(t *testing.T) {
		t.Parallel()
		repo := newFakeCandidateRepository()
		repo.getErr = errors.New("boom")

		svc := NewProfileService(repo)
		_, err := svc.GetProfile(context.Background(), "cand-1")

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to get profile")
	})
}

func TestProfileService_UpdateProfile(t *testing.T) {
	t.Parallel()

	t.Run("rejects empty ID", func(t *testing.T) {
		t.Parallel()
		svc := NewProfileService(newFakeCandidateRepository())
		err := svc.UpdateProfile(context.Background(), &domain.Candidate{})
		assert.ErrorContains(t, err, "profile ID is required")
	})

	t.Run("repo error is wrapped", func(t *testing.T) {
		t.Parallel()
		repo := newFakeCandidateRepository()
		repo.updateErr = errors.New("boom")

		svc := NewProfileService(repo)
		err := svc.UpdateProfile(context.Background(), &domain.Candidate{ID: "cand-1"})

		assert.ErrorContains(t, err, "failed to update profile")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		repo := newFakeCandidateRepository()
		svc := NewProfileService(repo)

		err := svc.UpdateProfile(context.Background(), &domain.Candidate{ID: "cand-1", Summary: "updated"})

		require.NoError(t, err)
		assert.Equal(t, "updated", repo.candidates["cand-1"].Summary)
	})
}

func TestProfileService_DeleteProfile(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		repo := newFakeCandidateRepository()
		require.NoError(t, repo.Save(context.Background(), &domain.Candidate{ID: "cand-1"}))

		svc := NewProfileService(repo)
		err := svc.DeleteProfile(context.Background(), "cand-1")

		require.NoError(t, err)
		_, ok := repo.candidates["cand-1"]
		assert.False(t, ok)
	})

	t.Run("repo error is wrapped", func(t *testing.T) {
		t.Parallel()
		repo := newFakeCandidateRepository()
		repo.deleteErr = errors.New("boom")

		svc := NewProfileService(repo)
		err := svc.DeleteProfile(context.Background(), "cand-1")

		assert.ErrorContains(t, err, "failed to delete profile")
	})
}
