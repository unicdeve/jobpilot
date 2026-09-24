//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// TestAgentRunRepository_ListSummaries_NamesWhatEachRunServed covers the
// activity list: every kind of run in one list, newest first, each named by
// its scout or its application's job, with its step count.
func TestAgentRunRepository_ListSummaries_NamesWhatEachRunServed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	candidate := saveTestCandidate(t, db)
	job := &domain.Job{CanonicalURL: "https://acme.example/job-1", Title: "Backend Engineer", CompanyName: "Acme"}
	require.NoError(t, NewJobRepository(db.Conn()).Save(ctx, job))
	app := saveTestApplication(t, db, candidate.ID, job.ID)
	scout := &domain.Scout{
		Name: "Daily Go", SavedSearchID: seedSavedSearch(t, db).ID,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true,
	}
	require.NoError(t, NewScoutRepository(db.Conn()).Save(ctx, scout))

	runs := NewAgentRunRepository(db.Conn())
	events := NewAgentEventRepository(db.Conn())
	start := time.Now().Add(-time.Hour)

	prepare := &domain.AgentRun{
		ApplicationID: app.ID, TaskType: domain.AgentTaskPrepareApplication,
		Status: domain.AgentRunStatusBlocked, StartedAt: start,
	}
	require.NoError(t, runs.Save(ctx, prepare))
	scoutRun := &domain.AgentRun{
		ScoutID: scout.ID, TaskType: domain.AgentTaskScoutRun,
		Status: domain.AgentRunStatusCompleted, StartedAt: start.Add(time.Minute),
	}
	require.NoError(t, runs.Save(ctx, scoutRun))
	for _, action := range []string{"scout_started", "jobs_collected", "scout_finished"} {
		require.NoError(t, events.Save(ctx, &domain.AgentEvent{RunID: scoutRun.ID, Action: action}))
	}

	page, err := runs.ListSummaries(ctx, domain.AgentRunFilter{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, page.Total)
	require.Len(t, page.Runs, 2)

	assert.Equal(t, scoutRun.ID, page.Runs[0].ID, "newest first")
	assert.Equal(t, "Daily Go", page.Runs[0].ScoutName)
	assert.Equal(t, scout.ID, page.Runs[0].ScoutID)
	assert.Equal(t, 3, page.Runs[0].EventCount)

	assert.Equal(t, prepare.ID, page.Runs[1].ID)
	assert.Equal(t, "Backend Engineer", page.Runs[1].JobTitle)
	assert.Equal(t, "Acme", page.Runs[1].Company)
	assert.Equal(t, 0, page.Runs[1].EventCount)

	t.Run("filters by kind and status", func(t *testing.T) {
		page, err := runs.ListSummaries(ctx, domain.AgentRunFilter{TaskType: domain.AgentTaskScoutRun, Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, 1, page.Total)
		require.Len(t, page.Runs, 1)
		assert.Equal(t, scoutRun.ID, page.Runs[0].ID)

		page, err = runs.ListSummaries(ctx, domain.AgentRunFilter{Status: domain.AgentRunStatusBlocked, Limit: 10})
		require.NoError(t, err)
		require.Len(t, page.Runs, 1)
		assert.Equal(t, prepare.ID, page.Runs[0].ID)
	})

	t.Run("pages, and reports the total across pages", func(t *testing.T) {
		page, err := runs.ListSummaries(ctx, domain.AgentRunFilter{Limit: 1, Offset: 1})
		require.NoError(t, err)
		assert.Equal(t, 2, page.Total)
		require.Len(t, page.Runs, 1)
		assert.Equal(t, prepare.ID, page.Runs[0].ID)
	})

	t.Run("gets one run the same way", func(t *testing.T) {
		got, err := runs.GetSummary(ctx, scoutRun.ID)
		require.NoError(t, err)
		assert.Equal(t, "Daily Go", got.ScoutName)
		assert.Equal(t, 3, got.EventCount)
	})
}

// A deleted scout must not take the record of what it did with it.
func TestAgentRunRepository_ListSummaries_KeepsRunsOfADeletedScout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	scouts := NewScoutRepository(db.Conn())
	scout := &domain.Scout{
		Name: "Daily Go", SavedSearchID: seedSavedSearch(t, db).ID,
		MaxApplicationsPerRun: 5, Source: domain.ScoutSourceLocal, IsEnabled: true,
	}
	require.NoError(t, scouts.Save(ctx, scout))
	runs := NewAgentRunRepository(db.Conn())
	run := &domain.AgentRun{ScoutID: scout.ID, TaskType: domain.AgentTaskScoutRun, Status: domain.AgentRunStatusCompleted}
	require.NoError(t, runs.Save(ctx, run))

	require.NoError(t, scouts.Delete(ctx, scout.ID))

	got, err := runs.GetSummary(ctx, run.ID)
	require.NoError(t, err)
	assert.Empty(t, got.ScoutID, "the link is cleared, not the run")
	assert.Empty(t, got.ScoutName)
}

func TestAgentRunRepository_GetSummary_NotFound(t *testing.T) {
	t.Parallel()

	_, err := NewAgentRunRepository(newTestDB(t).Conn()).GetSummary(context.Background(), "does-not-exist")
	assert.ErrorIs(t, err, ports.ErrAgentRunNotFound)
}

// TestMigration00012_AgentActivity migrates a database holding trails
// written before 00012 and checks what the migration did to them: scout
// runs linked to their scout, filled answers removed, and scout decisions
// given the policy result they never had.
func TestMigration00012_AgentActivity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := NewDB(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	provider, err := db.migrationProvider()
	require.NoError(t, err)
	_, err = provider.UpTo(ctx, 11)
	require.NoError(t, err)

	search := seedSavedSearch(t, db)
	conn := db.Conn()
	exec := func(query string, args ...any) {
		t.Helper()
		_, err := conn.ExecContext(ctx, query, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO scouts (id, name, saved_search_id) VALUES ('scout-1', 'Daily Go', ?)`, search.ID)
	exec(`INSERT INTO agent_runs (id, task_type, status) VALUES ('run-scout', 'scout_run', 'completed')`)
	exec(`INSERT INTO agent_runs (id, task_type, status) VALUES ('run-browser', 'prepare_application', 'completed')`)
	for _, row := range [][]string{
		{"e1", "run-scout", "scout_started", `scout "Daily Go", search "Go roles"`, "floor 70, ceiling 5, source local"},
		{"e2", "run-scout", "job_rejected", "Frontend Engineer", "match score 40 is below 70"},
		{"e3", "run-scout", "application_created", "Backend Engineer", "application a-1, eligible"},
		{"e4", "run-browser", "fill_field", "Email", "jane@example.com"},
		{"e5", "run-browser", "skip_field", "Visa status", `"Citizen" is not one of the choices offered`},
		{"e6", "run-browser", "resolve_question", "Email", "contact"},
	} {
		exec(`INSERT INTO agent_events (id, run_id, action, input_summary, output_summary, duration_ms) VALUES (?, ?, ?, ?, ?, 0)`,
			row[0], row[1], row[2], row[3], row[4])
	}

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	run, err := NewAgentRunRepository(conn).GetSummary(ctx, "run-scout")
	require.NoError(t, err)
	assert.Equal(t, "scout-1", run.ScoutID)
	assert.Equal(t, "Daily Go", run.ScoutName)

	byID := map[string]*domain.AgentEvent{}
	for _, runID := range []string{"run-scout", "run-browser"} {
		list, err := NewAgentEventRepository(conn).ListByRun(ctx, runID)
		require.NoError(t, err)
		for _, event := range list {
			byID[event.ID] = event
		}
	}

	assert.Empty(t, byID["e4"].OutputSummary, "the answer is gone")
	assert.Equal(t, "the answer is not one of the choices offered", byID["e5"].OutputSummary, "so is the rejected one")
	assert.Equal(t, "contact", byID["e6"].OutputSummary, "other steps are untouched")

	assert.Equal(t, domain.PolicyDenied, byID["e2"].PolicyResult)
	assert.Equal(t, domain.PolicyAllowed, byID["e3"].PolicyResult)
	assert.Empty(t, byID["e1"].PolicyResult)

	_, err = provider.Down(ctx)
	require.NoError(t, err, "00012 must migrate down")
}
