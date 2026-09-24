package ports

import (
	"context"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// AgentRunRepository persists and retrieves agent task runs
// (docs/02_features/12_agent_activity.md).
type AgentRunRepository interface {
	Save(ctx context.Context, run *domain.AgentRun) error
	Update(ctx context.Context, run *domain.AgentRun) error
	GetByID(ctx context.Context, id string) (*domain.AgentRun, error)
	ListByApplication(ctx context.Context, applicationID string) ([]*domain.AgentRun, error)
	// ListUnattached returns runs that belong to no single application,
	// newest first. A scout run creates several applications, so it
	// belongs to none of them (ADR-0018 §4).
	ListUnattached(ctx context.Context, limit int) ([]*domain.AgentRun, error)
}

// AgentEventRepository persists and retrieves the structured steps within
// an AgentRun.
type AgentEventRepository interface {
	Save(ctx context.Context, event *domain.AgentEvent) error
	ListByRun(ctx context.Context, runID string) ([]*domain.AgentEvent, error)
}
