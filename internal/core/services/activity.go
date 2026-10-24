package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// Activity list page sizes.
const (
	defaultActivityPageSize = 50
	maxActivityPageSize     = 200
)

// ErrInvalidActivityFilter is returned for a filter naming a task type or
// status that does not exist. Such a filter would match nothing, which
// reads as "no activity" rather than as the mistake it is.
var ErrInvalidActivityFilter = errors.New("invalid activity filter")

// AgentActivityService reads the trail every agent run leaves, across all
// kinds of run (docs/02_features/12_agent_activity.md).
//
// It only reads, but for closing the runs a stopped daemon left open. The
// runs are written by the services that do the work — BrowserWorkerService
// and ScoutService — and nothing here can change what they recorded.
type AgentActivityService struct {
	runs   ports.AgentRunRepository
	events ports.AgentEventRepository
}

// NewAgentActivityService constructs an AgentActivityService.
func NewAgentActivityService(runs ports.AgentRunRepository, events ports.AgentEventRepository) *AgentActivityService {
	return &AgentActivityService{runs: runs, events: events}
}

// CloseUnfinishedRuns ends every run still recorded as in progress, as
// failed, and reports how many it closed. It is called when a workspace
// opens: runs only exist inside a running daemon, so one still open then
// was cut off by a daemon that stopped without closing it, and nothing will
// ever finish it (ADR-0033 §5).
func (s *AgentActivityService) CloseUnfinishedRuns(ctx context.Context) (int, error) {
	return s.runs.CloseUnfinished(ctx, "JobPilot stopped while this run was in progress", time.Now())
}

// ListRuns returns one page of runs, newest first.
func (s *AgentActivityService) ListRuns(ctx context.Context, filter domain.AgentRunFilter) (*domain.AgentRunPage, error) {
	if filter.TaskType != "" && !domain.ValidAgentTaskType(filter.TaskType) {
		return nil, fmt.Errorf("%w: unknown task type %q", ErrInvalidActivityFilter, filter.TaskType)
	}
	if filter.Status != "" && !domain.ValidAgentRunStatus(filter.Status) {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidActivityFilter, filter.Status)
	}
	if filter.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must not be negative", ErrInvalidActivityFilter)
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultActivityPageSize
	}
	filter.Limit = min(filter.Limit, maxActivityPageSize)

	return s.runs.ListSummaries(ctx, filter)
}

// GetRun returns one run with every step it recorded, in order.
func (s *AgentActivityService) GetRun(ctx context.Context, id string) (*domain.AgentRunDetail, error) {
	run, err := s.runs.GetSummary(ctx, id)
	if err != nil {
		return nil, err
	}
	events, err := s.events.ListByRun(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list the run's steps: %w", err)
	}
	return &domain.AgentRunDetail{Run: run, Events: events}, nil
}
