package domain

import "time"

// AgentRunStatus is the lifecycle state of a single autonomous task run
// (e.g. one Scout run).
type AgentRunStatus string

// Agent run statuses.
const (
	AgentRunStatusRunning   AgentRunStatus = "running"
	AgentRunStatusCompleted AgentRunStatus = "completed"
	AgentRunStatusFailed    AgentRunStatus = "failed"
)

// InProgress reports whether a run with this status has not finished yet.
func (s AgentRunStatus) InProgress() bool {
	return s == AgentRunStatusRunning
}

// RunTrigger is what started a run (ADR-0027 §9).
type RunTrigger string

// Run triggers.
const (
	// RunTriggerManual is a run the user started.
	RunTriggerManual RunTrigger = "manual"
	// RunTriggerSchedule is a run a Scout's schedule started.
	RunTriggerSchedule RunTrigger = "schedule"
)

// AgentRun is one execution of an agent task (docs/02_features/12_agent_activity.md).
type AgentRun struct {
	ID string `json:"id"`
	// ScoutID is the scout that ran, cleared when the scout is deleted:
	// the run outlives it as the record of what it did.
	ScoutID string `json:"scout_id,omitempty"`
	// Trigger is what started the run: the user, or its scout's schedule.
	Trigger     RunTrigger     `json:"trigger,omitempty"`
	TaskType    string         `json:"task_type"`
	Status      AgentRunStatus `json:"status"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Error       string         `json:"error,omitempty"`
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

// Agent task types: what a run was doing. A Scout run is the only kind
// since ADR-0034 removed preparing an application in a browser.
const (
	AgentTaskScoutRun = "scout_run"
)

// Policy results an AgentEvent can record. A policy result is the outcome
// of a deterministic rule the run was held to — a scout's eligibility
// rules — and the reason stays in the event's output summary.
const (
	PolicyAllowed = "allowed"
	PolicyDenied  = "denied"
)

// ValidAgentRunStatus reports whether s is a known run status.
func ValidAgentRunStatus(s AgentRunStatus) bool {
	switch s {
	case AgentRunStatusRunning, AgentRunStatusCompleted, AgentRunStatusFailed:
		return true
	}
	return false
}

// ValidAgentTaskType reports whether t is a known task type.
func ValidAgentTaskType(t string) bool {
	return t == AgentTaskScoutRun
}

// AgentRunFilter narrows the activity list. A zero value lists every run.
type AgentRunFilter struct {
	TaskType string
	Status   AgentRunStatus
	Limit    int
	Offset   int
}

// AgentRunSummary is one row of the activity list: a run, and enough about
// what it served to name it without a second request.
type AgentRunSummary struct {
	AgentRun
	// ScoutName is set for a scout run whose scout still exists.
	ScoutName  string `json:"scout_name,omitempty"`
	EventCount int    `json:"event_count"`
}

// AgentRunPage is one page of the activity list, newest first.
type AgentRunPage struct {
	Runs  []*AgentRunSummary `json:"runs"`
	Total int                `json:"total"`
}

// AgentRunDetail is one run with every step it recorded, in order.
type AgentRunDetail struct {
	Run    *AgentRunSummary `json:"run"`
	Events []*AgentEvent    `json:"events"`
}
