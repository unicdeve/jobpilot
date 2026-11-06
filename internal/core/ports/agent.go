package ports

import (
	"context"
	"errors"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// ErrAgentRunNotFound is returned when no run has the requested ID.
var ErrAgentRunNotFound = errors.New("agent run not found")

// AgentRunRepository persists and retrieves agent task runs
// (docs/02_features/12_agent_activity.md).
type AgentRunRepository interface {
	Save(ctx context.Context, run *domain.AgentRun) error
	Update(ctx context.Context, run *domain.AgentRun) error
	GetByID(ctx context.Context, id string) (*domain.AgentRun, error)
	// List returns runs newest first.
	List(ctx context.Context, limit int) ([]*domain.AgentRun, error)
	// ListSummaries returns one page of runs, newest first, each named by
	// the scout that ran (docs/02_features/12_agent_activity.md).
	// The service sets the filter's limit; the repository applies it.
	ListSummaries(ctx context.Context, filter domain.AgentRunFilter) (*domain.AgentRunPage, error)
	// GetSummary returns one run, named the same way.
	GetSummary(ctx context.Context, id string) (*domain.AgentRunSummary, error)
	// LastScoutStart returns when a scout's most recent run started, or
	// nil if it has never run.
	LastScoutStart(ctx context.Context, scoutID string) (*time.Time, error)
	// CloseUnfinished ends every run still in progress as failed, with
	// reason, at the given time, and reports how many it closed.
	CloseUnfinished(ctx context.Context, reason string, at time.Time) (int, error)
}

// AgentEventRepository persists and retrieves the structured steps within
// an AgentRun.
type AgentEventRepository interface {
	Save(ctx context.Context, event *domain.AgentEvent) error
	ListByRun(ctx context.Context, runID string) ([]*domain.AgentEvent, error)
}
