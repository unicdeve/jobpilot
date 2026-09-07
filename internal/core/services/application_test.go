package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestApplicationService_CreateApplication(t *testing.T) {
	t.Parallel()

	t.Run("defaults to review mode and DISCOVERED state", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		svc := NewApplicationService(repo)

		app, err := svc.CreateApplication(context.Background(), "job-1", "cand-1", "", "")

		require.NoError(t, err)
		assert.Equal(t, domain.ApplicationModeReview, app.Mode)
		assert.Equal(t, domain.ApplicationStateDiscovered, app.State)
		assert.NotEmpty(t, app.ID)
	})

	t.Run("save error propagates", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		repo.saveErr = errors.New("disk full")
		svc := NewApplicationService(repo)

		_, err := svc.CreateApplication(context.Background(), "job-1", "cand-1", "", domain.ApplicationModeAssist)

		assert.ErrorContains(t, err, "failed to create application")
	})
}

func TestApplicationService_TransitionState(t *testing.T) {
	t.Parallel()

	validTransitions := []struct {
		from, to domain.ApplicationState
	}{
		{domain.ApplicationStateDiscovered, domain.ApplicationStateMatched},
		{domain.ApplicationStateMatched, domain.ApplicationStatePreparing},
		{domain.ApplicationStatePreparing, domain.ApplicationStateReadyForReview},
		{domain.ApplicationStateReadyForReview, domain.ApplicationStateApproved},
		{domain.ApplicationStateReadyForReview, domain.ApplicationStateRejected},
		{domain.ApplicationStateApproved, domain.ApplicationStateSubmitted},
		{domain.ApplicationStateSubmitted, domain.ApplicationStateRejected},
		{domain.ApplicationStateFailed, domain.ApplicationStatePreparing},
		{domain.ApplicationStateDiscovered, domain.ApplicationStateWithdrawn},
	}
	for _, tt := range validTransitions {
		t.Run(string(tt.from)+"->"+string(tt.to)+" is allowed", func(t *testing.T) {
			t.Parallel()
			repo := newFakeApplicationRepository()
			svc := NewApplicationService(repo)
			app := &domain.Application{ID: "app-1", State: tt.from}
			require.NoError(t, repo.Save(context.Background(), app))

			got, err := svc.TransitionState(context.Background(), "app-1", tt.to, "reason")

			require.NoError(t, err)
			assert.Equal(t, tt.to, got.State)
			assert.Equal(t, tt.to, repo.applications["app-1"].State, "repository must reflect the new state")
		})
	}

	invalidTransitions := []struct {
		from, to domain.ApplicationState
	}{
		{domain.ApplicationStateDiscovered, domain.ApplicationStateSubmitted},
		{domain.ApplicationStateDiscovered, domain.ApplicationStateApproved},
		{domain.ApplicationStateMatched, domain.ApplicationStateSubmitted},
		{domain.ApplicationStateSubmitted, domain.ApplicationStateApproved},
		{domain.ApplicationStateRejected, domain.ApplicationStateMatched},
		{domain.ApplicationStateWithdrawn, domain.ApplicationStatePreparing},
	}
	for _, tt := range invalidTransitions {
		t.Run(string(tt.from)+"->"+string(tt.to)+" is rejected", func(t *testing.T) {
			t.Parallel()
			repo := newFakeApplicationRepository()
			svc := NewApplicationService(repo)
			app := &domain.Application{ID: "app-1", State: tt.from}
			require.NoError(t, repo.Save(context.Background(), app))

			_, err := svc.TransitionState(context.Background(), "app-1", tt.to, "reason")

			assert.ErrorContains(t, err, "cannot transition application")
			assert.Equal(t, tt.from, repo.applications["app-1"].State, "state must be unchanged after a rejected transition")
			assert.Empty(t, repo.transitions["app-1"], "no history should be recorded for a rejected transition")
		})
	}

	t.Run("application fetch error propagates", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		svc := NewApplicationService(repo)

		_, err := svc.TransitionState(context.Background(), "missing-app", domain.ApplicationStateMatched, "")

		assert.ErrorContains(t, err, "failed to get application")
	})

	t.Run("repository transition error propagates", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		repo.transitionErr = errors.New("concurrent update")
		svc := NewApplicationService(repo)
		app := &domain.Application{ID: "app-1", State: domain.ApplicationStateDiscovered}
		require.NoError(t, repo.Save(context.Background(), app))

		_, err := svc.TransitionState(context.Background(), "app-1", domain.ApplicationStateMatched, "")

		assert.ErrorContains(t, err, "failed to transition application state")
	})
}

func TestApplicationService_ListApplicationsAndHistory(t *testing.T) {
	t.Parallel()

	repo := newFakeApplicationRepository()
	svc := NewApplicationService(repo)
	app := &domain.Application{ID: "app-1", CandidateID: "cand-1", State: domain.ApplicationStateDiscovered}
	require.NoError(t, repo.Save(context.Background(), app))
	require.NoError(t, repo.Save(context.Background(), &domain.Application{ID: "app-2", CandidateID: "cand-2", State: domain.ApplicationStateDiscovered}))

	apps, err := svc.ListApplications(context.Background(), "cand-1")
	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.Equal(t, "app-1", apps[0].ID)

	_, err = svc.TransitionState(context.Background(), "app-1", domain.ApplicationStateMatched, "matched via search")
	require.NoError(t, err)

	history, err := svc.ListHistory(context.Background(), "app-1")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "matched via search", history[0].Reason)
}
