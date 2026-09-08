package domain

import "time"

// AgentRunStatus is the lifecycle state of a single autonomous task run
// (e.g. one "prepare this application in a browser" attempt).
type AgentRunStatus string

// Agent run statuses.
const (
	AgentRunStatusRunning   AgentRunStatus = "running"
	AgentRunStatusCompleted AgentRunStatus = "completed"
	AgentRunStatusBlocked   AgentRunStatus = "blocked"
	AgentRunStatusFailed    AgentRunStatus = "failed"
)

// AgentRun is one execution of an agent task (docs/02_features/12_agent_activity.md),
// scoped to the application it was working on.
type AgentRun struct {
	ID            string         `json:"id"`
	ApplicationID string         `json:"application_id"`
	TaskType      string         `json:"task_type"`
	Status        AgentRunStatus `json:"status"`
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	Error         string         `json:"error,omitempty"`
}

// AgentEvent is one structured step within an AgentRun — enough to show
// transparent activity without exposing hidden chain-of-thought, per
// docs/02_features/12_agent_activity.md.
type AgentEvent struct {
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	Action        string    `json:"action"`
	Tool          string    `json:"tool"`
	Status        string    `json:"status"`
	InputSummary  string    `json:"input_summary,omitempty"`
	OutputSummary string    `json:"output_summary,omitempty"`
	PolicyResult  string    `json:"policy_result,omitempty"`
	DurationMs    int64     `json:"duration_ms"`
	CreatedAt     time.Time `json:"created_at"`
}
