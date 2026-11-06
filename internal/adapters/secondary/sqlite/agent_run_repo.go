package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// AgentRunRepository is a ports.AgentRunRepository backed by SQLite.
type AgentRunRepository struct {
	db *sql.DB
}

// NewAgentRunRepository constructs an AgentRunRepository over db.
func NewAgentRunRepository(db *sql.DB) *AgentRunRepository {
	return &AgentRunRepository{db: db}
}

// Save inserts a new agent run, generating an ID and start time if unset.
func (r *AgentRunRepository) Save(ctx context.Context, run *domain.AgentRun) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now()
	}

	if run.Trigger == "" {
		run.Trigger = domain.RunTriggerManual
	}

	query := `INSERT INTO agent_runs (
		id, scout_id, started_by, task_type, status, started_at, completed_at, error
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		run.ID, nullableString(run.ScoutID), string(run.Trigger),
		run.TaskType, run.Status,
		run.StartedAt, run.CompletedAt, nullableString(run.Error),
	)
	if err != nil {
		return fmt.Errorf("failed to save agent run: %w", err)
	}

	return nil
}

// Update persists an agent run's mutable fields (status, completion time,
// error) as it progresses or finishes.
func (r *AgentRunRepository) Update(ctx context.Context, run *domain.AgentRun) error {
	query := `UPDATE agent_runs SET status = ?, completed_at = ?, error = ? WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, run.Status, run.CompletedAt, nullableString(run.Error), run.ID)
	if err != nil {
		return fmt.Errorf("failed to update agent run: %w", err)
	}

	return nil
}

const agentRunColumns = `
	id, scout_id, started_by, task_type, status, started_at, completed_at, error`

func scanAgentRun(scanner interface{ Scan(...any) error }) (*domain.AgentRun, error) {
	var run domain.AgentRun
	if err := scanner.Scan(agentRunDest(&run)...); err != nil {
		return nil, err
	}
	return &run, nil
}

// agentRunDest returns scan destinations for agentRunColumns, in order,
// with the nullable text columns read through sql.NullString into run.
func agentRunDest(run *domain.AgentRun) []any {
	return []any{
		&run.ID,
		nullStringInto(&run.ScoutID),
		(*string)(&run.Trigger),
		nullStringInto(&run.TaskType),
		nullStringInto((*string)(&run.Status)),
		&run.StartedAt,
		&run.CompletedAt,
		nullStringInto(&run.Error),
	}
}

// GetByID retrieves an agent run by ID.
func (r *AgentRunRepository) GetByID(ctx context.Context, id string) (*domain.AgentRun, error) {
	query := `SELECT` + agentRunColumns + ` FROM agent_runs WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	run, err := scanAgentRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ports.NotFoundError{Kind: "agent run", ID: id}
		}
		return nil, fmt.Errorf("failed to get agent run: %w", err)
	}

	return run, nil
}

// List returns runs newest first.
func (r *AgentRunRepository) List(ctx context.Context, limit int) ([]*domain.AgentRun, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT` + agentRunColumns + `
		FROM agent_runs ORDER BY started_at DESC LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runs := make([]*domain.AgentRun, 0)
	for rows.Next() {
		run, err := scanAgentRun(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read agent runs: %w", err)
	}
	return runs, nil
}

// LastScoutStart returns when the scout's most recent run started, or nil
// if it has never run. Whether a scheduled scout is due is measured from
// it (ADR-0027 §2).
func (r *AgentRunRepository) LastScoutStart(ctx context.Context, scoutID string) (*time.Time, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT started_at FROM agent_runs WHERE scout_id = ? ORDER BY started_at DESC LIMIT 1`, scoutID)
	var started time.Time
	err := row.Scan(&started)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the scout's last run: %w", err)
	}
	return &started, nil
}

// CloseUnfinished ends every run still running.
func (r *AgentRunRepository) CloseUnfinished(ctx context.Context, reason string, at time.Time) (int, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE agent_runs SET status = ?, completed_at = ?, error = ? WHERE status = ?`,
		domain.AgentRunStatusFailed, at, reason, domain.AgentRunStatusRunning,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to close unfinished agent runs: %w", err)
	}
	closed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to count closed agent runs: %w", err)
	}
	return int(closed), nil
}

// agentRunSummaryQuery selects a run with the scout that started it. It is
// a LEFT JOIN, so a run whose scout has since been deleted is still listed.
const agentRunSummaryQuery = `SELECT
	r.id, r.scout_id, r.started_by, r.task_type, r.status, r.started_at, r.completed_at, r.error,
	s.name,
	(SELECT COUNT(*) FROM agent_events e WHERE e.run_id = r.id)
FROM agent_runs r
LEFT JOIN scouts s ON s.id = r.scout_id`

func scanAgentRunSummary(scanner interface{ Scan(...any) error }) (*domain.AgentRunSummary, error) {
	var summary domain.AgentRunSummary
	dest := append(agentRunDest(&summary.AgentRun),
		nullStringInto(&summary.ScoutName),
		&summary.EventCount,
	)
	if err := scanner.Scan(dest...); err != nil {
		return nil, err
	}
	return &summary, nil
}

// ListSummaries returns one page of runs matching filter, newest first,
// with the total number that match.
func (r *AgentRunRepository) ListSummaries(ctx context.Context, filter domain.AgentRunFilter) (*domain.AgentRunPage, error) {
	where, args := agentRunWhere(filter)

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runs r`+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count agent runs: %w", err)
	}

	// id breaks ties so that paging is stable between runs that started in
	// the same instant.
	query := agentRunSummaryQuery + where + ` ORDER BY r.started_at DESC, r.id DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	page := &domain.AgentRunPage{Runs: make([]*domain.AgentRunSummary, 0), Total: total}
	for rows.Next() {
		summary, err := scanAgentRunSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent run: %w", err)
		}
		page.Runs = append(page.Runs, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read agent runs: %w", err)
	}
	return page, nil
}

// GetSummary returns one run with what it served.
func (r *AgentRunRepository) GetSummary(ctx context.Context, id string) (*domain.AgentRunSummary, error) {
	row := r.db.QueryRowContext(ctx, agentRunSummaryQuery+` WHERE r.id = ?`, id)
	summary, err := scanAgentRunSummary(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ports.ErrAgentRunNotFound, id)
		}
		return nil, fmt.Errorf("failed to get agent run: %w", err)
	}
	return summary, nil
}

func agentRunWhere(filter domain.AgentRunFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.TaskType != "" {
		clauses = append(clauses, "r.task_type = ?")
		args = append(args, filter.TaskType)
	}
	if filter.Status != "" {
		clauses = append(clauses, "r.status = ?")
		args = append(args, filter.Status)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
