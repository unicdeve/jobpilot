package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func TestApplicationService_CreateApplication(t *testing.T) {
	t.Parallel()

	t.Run("defaults to review mode and DISCOVERED state", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		svc := NewApplicationService(repo, jobsWith("job-1"), candidatesWith("cand-1"), newFakeResumeRepository())

		app, err := svc.CreateApplication(context.Background(), "job-1", "cand-1", "", "")

		require.NoError(t, err)
		assert.Equal(t, domain.ApplicationModeReview, app.Mode)
		assert.Equal(t, domain.ApplicationStateDiscovered, app.State)
		assert.NotEmpty(t, app.ID)
	})

	t.Run("a mode JobPilot does not have is refused rather than stored", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		svc := NewApplicationService(repo, jobsWith("job-1"), candidatesWith("cand-1"), newFakeResumeRepository())

		// The column is free TEXT, so without this check any string a
		// caller sent became a mode — and "autopilot", removed in
		// ADR-0014, could come back through the CLI flag alone.
		for _, mode := range []domain.ApplicationMode{"autopilot", "reveiw", "ASSIST"} {
			_, err := svc.CreateApplication(context.Background(), "job-1", "cand-1", "", mode)

			assert.ErrorContains(t, err, "unknown application mode", "mode %q", mode)
		}
		assert.Empty(t, repo.applications, "nothing should have been written")
	})

	t.Run("a job, candidate or resume that does not exist is refused before anything is saved", func(t *testing.T) {
		t.Parallel()

		// Each of these reached the database's foreign key, which failed as
		// a 500 that did not say which of the three was missing.
		for _, tc := range []struct {
			name                      string
			jobID, candidateID, resID string
			missing                   string
		}{
			{"job", "job-404", "cand-1", "", "job not found: job-404"},
			{"candidate", "job-1", "cand-404", "", "candidate not found: cand-404"},
			{"resume", "job-1", "cand-1", "res-404", "resume not found: res-404"},
		} {
			repo := newFakeApplicationRepository()
			svc := NewApplicationService(repo, jobsWith("job-1"), candidatesWith("cand-1"), newFakeResumeRepository())

			_, err := svc.CreateApplication(context.Background(), tc.jobID, tc.candidateID, tc.resID, "")

			require.ErrorIs(t, err, ports.ErrUnknownReference, tc.name)
			assert.NotErrorIs(t, err, ports.ErrNotFound, "%s: the application being created is not what is missing", tc.name)
			assert.ErrorContains(t, err, tc.missing, tc.name)
			assert.Empty(t, repo.applications, "%s: nothing should have been written", tc.name)
		}
	})

	t.Run("a resume that exists is accepted", func(t *testing.T) {
		t.Parallel()
		resumes := newFakeResumeRepository()
		resume := &domain.Resume{CandidateID: "cand-1"}
		require.NoError(t, resumes.Save(context.Background(), resume))
		svc := NewApplicationService(newFakeApplicationRepository(), jobsWith("job-1"), candidatesWith("cand-1"), resumes)

		app, err := svc.CreateApplication(context.Background(), "job-1", "cand-1", resume.ID, "")

		require.NoError(t, err)
		assert.Equal(t, resume.ID, app.ResumeID)
	})

	t.Run("save error propagates", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		repo.saveErr = errors.New("disk full")
		svc := NewApplicationService(repo, jobsWith("job-1"), candidatesWith("cand-1"), newFakeResumeRepository())

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
		// Outcomes the user reports after submitting.
		{domain.ApplicationStateSubmitted, domain.ApplicationStateInterviewing},
		{domain.ApplicationStateSubmitted, domain.ApplicationStateOffered},
		{domain.ApplicationStateSubmitted, domain.ApplicationStateNoResponse},
		{domain.ApplicationStateInterviewing, domain.ApplicationStateOffered},
		{domain.ApplicationStateInterviewing, domain.ApplicationStateRejected},
		{domain.ApplicationStateInterviewing, domain.ApplicationStateNoResponse},
		{domain.ApplicationStateOffered, domain.ApplicationStateRejected},
		// A late reply reopens an application the user had given up on.
		{domain.ApplicationStateNoResponse, domain.ApplicationStateInterviewing},
		{domain.ApplicationStateNoResponse, domain.ApplicationStateRejected},
		// Withdrawing is possible at every point after submitting: people
		// pull out of processes, decline interviews and turn down offers.
		{domain.ApplicationStateSubmitted, domain.ApplicationStateWithdrawn},
		{domain.ApplicationStateInterviewing, domain.ApplicationStateWithdrawn},
		{domain.ApplicationStateOffered, domain.ApplicationStateWithdrawn},
	}
	for _, tt := range validTransitions {
		t.Run(string(tt.from)+"->"+string(tt.to)+" is allowed", func(t *testing.T) {
			t.Parallel()
			repo := newFakeApplicationRepository()
			svc := NewApplicationService(repo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
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
		// No outcome is a way back into the pipeline, and none of them is a
		// second route to SUBMITTED (AC-005).
		{domain.ApplicationStateInterviewing, domain.ApplicationStateSubmitted},
		{domain.ApplicationStateNoResponse, domain.ApplicationStateSubmitted},
		{domain.ApplicationStateOffered, domain.ApplicationStateInterviewing},
		{domain.ApplicationStateOffered, domain.ApplicationStateNoResponse},
		{domain.ApplicationStateInterviewing, domain.ApplicationStatePreparing},
		{domain.ApplicationStateApproved, domain.ApplicationStateInterviewing},
		{domain.ApplicationStateReadyForReview, domain.ApplicationStateOffered},
	}
	for _, tt := range invalidTransitions {
		t.Run(string(tt.from)+"->"+string(tt.to)+" is rejected", func(t *testing.T) {
			t.Parallel()
			repo := newFakeApplicationRepository()
			svc := NewApplicationService(repo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
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
		svc := NewApplicationService(repo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())

		_, err := svc.TransitionState(context.Background(), "missing-app", domain.ApplicationStateMatched, "")

		assert.ErrorContains(t, err, "failed to get application")
	})

	t.Run("repository transition error propagates", func(t *testing.T) {
		t.Parallel()
		repo := newFakeApplicationRepository()
		repo.transitionErr = errors.New("concurrent update")
		svc := NewApplicationService(repo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
		app := &domain.Application{ID: "app-1", State: domain.ApplicationStateDiscovered}
		require.NoError(t, repo.Save(context.Background(), app))

		_, err := svc.TransitionState(context.Background(), "app-1", domain.ApplicationStateMatched, "")

		assert.ErrorContains(t, err, "failed to transition application state")
	})
}

func TestApplicationService_ListApplicationsAndHistory(t *testing.T) {
	t.Parallel()

	repo := newFakeApplicationRepository()
	svc := NewApplicationService(repo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
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

func TestAllowedTransitions(t *testing.T) {
	t.Parallel()

	t.Run("offers what the rule table allows, for the client to show", func(t *testing.T) {
		t.Parallel()

		got := AllowedTransitions(&domain.Application{State: domain.ApplicationStateSubmitted})

		assert.Equal(t, []domain.ApplicationState{
			domain.ApplicationStateInterviewing,
			domain.ApplicationStateOffered,
			domain.ApplicationStateRejected,
			domain.ApplicationStateNoResponse,
			domain.ApplicationStateWithdrawn,
		}, got)
	})

	t.Run("a closed application offers nothing", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, AllowedTransitions(&domain.Application{State: domain.ApplicationStateRejected}))
		assert.Empty(t, AllowedTransitions(&domain.Application{State: domain.ApplicationStateWithdrawn}))
	})

	t.Run("every state it offers is one TransitionState actually accepts", func(t *testing.T) {
		t.Parallel()

		// The point of serving this list is that a client can trust it. If
		// the two ever disagreed, the UI would offer a button that errors.
		for state := range allowedTransitions {
			for _, to := range AllowedTransitions(&domain.Application{State: state}) {
				repo := newFakeApplicationRepository()
				svc := NewApplicationService(repo, newFakeJobRepository(), newFakeCandidateRepository(), newFakeResumeRepository())
				require.NoError(t, repo.Save(context.Background(), &domain.Application{ID: "app-1", State: state}))

				_, err := svc.TransitionState(context.Background(), "app-1", to, "")

				assert.NoErrorf(t, err, "%s -> %s was offered but refused", state, to)
			}
		}
	})

	t.Run("the caller cannot edit the rule table through the result", func(t *testing.T) {
		t.Parallel()

		got := AllowedTransitions(&domain.Application{State: domain.ApplicationStateApproved})
		got[0] = domain.ApplicationStateRejected

		assert.Equal(t,
			domain.ApplicationStateSubmitted,
			AllowedTransitions(&domain.Application{State: domain.ApplicationStateApproved})[0],
		)
	})
}
