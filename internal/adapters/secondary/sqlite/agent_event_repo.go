package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// AgentEventRepository is a ports.AgentEventRepository backed by SQLite.
type AgentEventRepository struct {
	db *sql.DB
}

// NewAgentEventRepository constructs an AgentEventRepository over db.
func NewAgentEventRepository(db *sql.DB) *AgentEventRepository {
	return &AgentEventRepository{db: db}
}

// Save inserts a new agent event, generating an ID and timestamp if unset.
func (r *AgentEventRepository) Save(ctx context.Context, event *domain.AgentEvent) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	query := `INSERT INTO agent_events (
		id, run_id, action, tool, status, input_summary, output_summary, policy_result, duration_ms, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		event.ID, event.RunID, event.Action, event.Tool, event.Status,
		nullableString(event.InputSummary), nullableString(event.OutputSummary), nullableString(event.PolicyResult),
		event.DurationMs, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save agent event: %w", err)
	}

	return nil
}

// ListByRun returns every event for an agent run, in the order they
// occurred.
func (r *AgentEventRepository) ListByRun(ctx context.Context, runID string) ([]*domain.AgentEvent, error) {
	query := `SELECT
		id, run_id, action, tool, status, input_summary, output_summary, policy_result, duration_ms, created_at
	FROM agent_events WHERE run_id = ? ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := make([]*domain.AgentEvent, 0)
	for rows.Next() {
		var e domain.AgentEvent
		var tool, status, inputSummary, outputSummary, policyResult sql.NullString
		if err := rows.Scan(&e.ID, &e.RunID, &e.Action, &tool, &status, &inputSummary, &outputSummary, &policyResult, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent event: %w", err)
		}
		e.Tool = tool.String
		e.Status = status.String
		e.InputSummary = inputSummary.String
		e.OutputSummary = outputSummary.String
		e.PolicyResult = policyResult.String
		events = append(events, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating agent events: %w", err)
	}

	return events, nil
}
