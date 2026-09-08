package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/jobpilot/internal/core/domain"
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

	query := `INSERT INTO agent_runs (
		id, application_id, task_type, status, started_at, completed_at, error
	) VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		run.ID, run.ApplicationID, run.TaskType, run.Status, run.StartedAt, run.CompletedAt, nullableString(run.Error),
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
	id, application_id, task_type, status, started_at, completed_at, error`

func scanAgentRun(scanner interface{ Scan(...any) error }) (*domain.AgentRun, error) {
	var run domain.AgentRun
	var applicationID, taskType, status, errMsg sql.NullString
	if err := scanner.Scan(&run.ID, &applicationID, &taskType, &status, &run.StartedAt, &run.CompletedAt, &errMsg); err != nil {
		return nil, err
	}
	run.ApplicationID = applicationID.String
	run.TaskType = taskType.String
	run.Status = domain.AgentRunStatus(status.String)
	run.Error = errMsg.String
	return &run, nil
}

// GetByID retrieves an agent run by ID.
func (r *AgentRunRepository) GetByID(ctx context.Context, id string) (*domain.AgentRun, error) {
	query := `SELECT` + agentRunColumns + ` FROM agent_runs WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	run, err := scanAgentRun(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("agent run not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get agent run: %w", err)
	}

	return run, nil
}

// ListByApplication returns every agent run for an application, most recent
// first.
func (r *AgentRunRepository) ListByApplication(ctx context.Context, applicationID string) ([]*domain.AgentRun, error) {
	query := `SELECT` + agentRunColumns + ` FROM agent_runs WHERE application_id = ? ORDER BY started_at DESC`

	rows, err := r.db.QueryContext(ctx, query, applicationID)
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
		return nil, fmt.Errorf("error iterating agent runs: %w", err)
	}

	return runs, nil
}
