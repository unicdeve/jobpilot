//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/unicdev/jobpilot/internal/core/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every list query must return a non-nil, empty slice when nothing
// matches, not nil: encoding/json marshals a nil slice as `null`, which
// breaks any client-side code that maps over or reads .length on the
// decoded result (e.g. the desktop app crashing on an empty Applications
// or Matches list).
func TestListQueries_ReturnEmptySliceNotNil_WhenNoRowsMatch(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	ctx := context.Background()

	jobs, err := NewJobRepository(db.Conn()).List(ctx, domain.JobFilter{}, 10, 0)
	require.NoError(t, err)
	assert.NotNil(t, jobs)
	assert.Empty(t, jobs)

	matches, err := NewMatchRepository(db.Conn()).ListByCandidate(ctx, candidate.ID)
	require.NoError(t, err)
	assert.NotNil(t, matches)
	assert.Empty(t, matches)

	resumes, err := NewResumeRepository(db.Conn()).GetByCandidateID(ctx, candidate.ID)
	require.NoError(t, err)
	assert.NotNil(t, resumes)
	assert.Empty(t, resumes)

	apps, err := NewApplicationRepository(db.Conn()).ListByCandidate(ctx, candidate.ID)
	require.NoError(t, err)
	assert.NotNil(t, apps)
	assert.Empty(t, apps)

	transitions, err := NewApplicationRepository(db.Conn()).ListStateTransitions(ctx, "nonexistent-app")
	require.NoError(t, err)
	assert.NotNil(t, transitions)
	assert.Empty(t, transitions)

	letters, err := NewCoverLetterRepository(db.Conn()).ListByApplication(ctx, "nonexistent-app")
	require.NoError(t, err)
	assert.NotNil(t, letters)
	assert.Empty(t, letters)

	questions, err := NewApplicationQuestionRepository(db.Conn()).ListByApplication(ctx, "nonexistent-app")
	require.NoError(t, err)
	assert.NotNil(t, questions)
	assert.Empty(t, questions)

	runs, err := NewAgentRunRepository(db.Conn()).List(ctx, 10)
	require.NoError(t, err)
	assert.NotNil(t, runs)
	assert.Empty(t, runs)
}
