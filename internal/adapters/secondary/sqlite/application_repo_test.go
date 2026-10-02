//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func saveTestApplication(t *testing.T, db *DB, candidateID, jobID string) *domain.Application {
	t.Helper()
	app := &domain.Application{
		JobID:       jobID,
		CandidateID: candidateID,
		Mode:        domain.ApplicationModeReview,
		State:       domain.ApplicationStateDiscovered,
	}
	require.NoError(t, NewApplicationRepository(db.Conn()).Save(context.Background(), app))
	return app
}

func TestApplicationRepository_SaveAndGetByID(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	repo := NewApplicationRepository(db.Conn())

	app := saveTestApplication(t, db, candidate.ID, job.ID)
	assert.NotEmpty(t, app.ID)

	got, err := repo.GetByID(context.Background(), app.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ApplicationStateDiscovered, got.State)
	assert.Equal(t, domain.ApplicationModeReview, got.Mode)
	assert.Empty(t, got.CurrentCoverLetterID, "unset nullable FK should round-trip as empty, not a literal string")
}

func TestApplicationRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewApplicationRepository(db.Conn())

	_, err := repo.GetByID(context.Background(), "does-not-exist")

	assert.ErrorContains(t, err, "application not found")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestApplicationRepository_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := saveTestJob(t, db, "https://acme.example/job-1")
	repo := NewApplicationRepository(db.Conn())
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	coverLetter := &domain.CoverLetter{ApplicationID: app.ID, Content: "draft"}
	require.NoError(t, NewCoverLetterRepository(db.Conn()).Save(context.Background(), coverLetter))

	app.CurrentCoverLetterID = coverLetter.ID
	app.ApprovalStatus = "pending"
	require.NoError(t, repo.Update(context.Background(), app))

	got, err := repo.GetByID(context.Background(), app.ID)
	require.NoError(t, err)
	assert.Equal(t, coverLetter.ID, got.CurrentCoverLetterID)
	assert.Equal(t, "pending", got.ApprovalStatus)
	assert.Equal(t, domain.ApplicationStateDiscovered, got.State, "Update must never change State")
}

func TestApplicationRepository_ListByCandidate(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate1 := saveTestCandidate(t, db)
	candidate2 := saveTestCandidate(t, db)
	job1 := saveTestJob(t, db, "https://acme.example/job-1")
	job2 := saveTestJob(t, db, "https://acme.example/job-2")
	repo := NewApplicationRepository(db.Conn())

	app1 := saveTestApplication(t, db, candidate1.ID, job1.ID)
	_ = saveTestApplication(t, db, candidate2.ID, job2.ID)

	apps, err := repo.ListByCandidate(context.Background(), candidate1.ID)

	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.Equal(t, app1.ID, apps[0].ID)
}

func TestApplicationRepository_TransitionState(t *testing.T) {
	t.Parallel()

	t.Run("valid transition updates state and records history", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		candidate := saveTestCandidate(t, db)
		job := saveTestJob(t, db, "https://acme.example/job-1")
		repo := NewApplicationRepository(db.Conn())
		app := saveTestApplication(t, db, candidate.ID, job.ID)

		err := repo.TransitionState(context.Background(), app.ID, domain.ApplicationStateDiscovered, domain.ApplicationStateMatched, "auto-matched")
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), app.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.ApplicationStateMatched, got.State)

		history, err := repo.ListStateTransitions(context.Background(), app.ID)
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.Equal(t, domain.ApplicationStateDiscovered, history[0].FromState)
		assert.Equal(t, domain.ApplicationStateMatched, history[0].ToState)
		assert.Equal(t, "auto-matched", history[0].Reason)
	})

	t.Run("stale from-state is rejected and nothing is recorded", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		candidate := saveTestCandidate(t, db)
		job := saveTestJob(t, db, "https://acme.example/job-1")
		repo := NewApplicationRepository(db.Conn())
		app := saveTestApplication(t, db, candidate.ID, job.ID)
		require.NoError(t, repo.TransitionState(context.Background(), app.ID, domain.ApplicationStateDiscovered, domain.ApplicationStateMatched, "first"))

		// Caller still thinks the application is DISCOVERED (stale read).
		err := repo.TransitionState(context.Background(), app.ID, domain.ApplicationStateDiscovered, domain.ApplicationStateMatched, "stale")

		assert.Error(t, err)

		got, err := repo.GetByID(context.Background(), app.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.ApplicationStateMatched, got.State, "state must be unchanged by the rejected transition")

		history, err := repo.ListStateTransitions(context.Background(), app.ID)
		require.NoError(t, err)
		assert.Len(t, history, 1, "the rejected transition must not add a history row")
	})

	t.Run("submitted_at is set only on first transition into SUBMITTED", func(t *testing.T) {
		t.Parallel()
		db := newTestDB(t)
		candidate := saveTestCandidate(t, db)
		job := saveTestJob(t, db, "https://acme.example/job-1")
		repo := NewApplicationRepository(db.Conn())
		app := saveTestApplication(t, db, candidate.ID, job.ID)
		ctx := context.Background()
		require.NoError(t, repo.TransitionState(ctx, app.ID, domain.ApplicationStateDiscovered, domain.ApplicationStateMatched, ""))
		require.NoError(t, repo.TransitionState(ctx, app.ID, domain.ApplicationStateMatched, domain.ApplicationStatePreparing, ""))
		require.NoError(t, repo.TransitionState(ctx, app.ID, domain.ApplicationStatePreparing, domain.ApplicationStateReadyForReview, ""))
		require.NoError(t, repo.TransitionState(ctx, app.ID, domain.ApplicationStateReadyForReview, domain.ApplicationStateApproved, ""))
		require.NoError(t, repo.TransitionState(ctx, app.ID, domain.ApplicationStateApproved, domain.ApplicationStateSubmitted, ""))

		got, err := repo.GetByID(ctx, app.ID)
		require.NoError(t, err)
		require.NotNil(t, got.SubmittedAt)
		firstSubmittedAt := *got.SubmittedAt

		require.NoError(t, repo.TransitionState(ctx, app.ID, domain.ApplicationStateSubmitted, domain.ApplicationStateRejected, "employer declined"))
		got, err = repo.GetByID(ctx, app.ID)
		require.NoError(t, err)
		require.NotNil(t, got.SubmittedAt)
		assert.Equal(t, firstSubmittedAt.Unix(), got.SubmittedAt.Unix(), "submitted_at must not be overwritten by a later transition")
	})
}
