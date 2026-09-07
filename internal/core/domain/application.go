package domain

import "time"

// ApplicationState is a step in an Application's lifecycle. Not every
// application progresses through every state (docs/02_features/08_applications.md).
type ApplicationState string

// Application lifecycle states, per docs/02_features/08_applications.md.
const (
	ApplicationStateDiscovered     ApplicationState = "DISCOVERED"
	ApplicationStateMatched        ApplicationState = "MATCHED"
	ApplicationStatePreparing      ApplicationState = "PREPARING"
	ApplicationStateReadyForReview ApplicationState = "READY_FOR_REVIEW"
	ApplicationStateApproved       ApplicationState = "APPROVED"
	ApplicationStateSubmitted      ApplicationState = "SUBMITTED"
	ApplicationStateRejected       ApplicationState = "REJECTED"
	ApplicationStateWithdrawn      ApplicationState = "WITHDRAWN"
	ApplicationStateFailed         ApplicationState = "FAILED"
)

// ApplicationMode is which of the three application modes (ADR-0007)
// governs an Application. Review is the default.
type ApplicationMode string

// Application modes, per ADR-0007. Review is the default.
const (
	ApplicationModeAssist    ApplicationMode = "assist"
	ApplicationModeReview    ApplicationMode = "review"
	ApplicationModeAutopilot ApplicationMode = "autopilot"
)

// Application tracks a candidate's progress applying to a specific job,
// distinct from the Job itself.
type Application struct {
	ID                   string           `json:"id"`
	JobID                string           `json:"job_id"`
	CandidateID          string           `json:"candidate_id"`
	ResumeID             string           `json:"resume_id"`
	Mode                 ApplicationMode  `json:"mode"`
	State                ApplicationState `json:"state"`
	CurrentCoverLetterID string           `json:"current_cover_letter_id"`
	PolicyResult         string           `json:"policy_result"`
	ApprovalStatus       string           `json:"approval_status"`
	SubmittedAt          *time.Time       `json:"submitted_at,omitempty"`
	FailureReason        string           `json:"failure_reason"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

// ApplicationStateTransition is one durable, historical record of an
// Application moving from one state to another
// (docs/02_features/08_applications.md AC-003), independent of the
// Application's current mutable State field.
type ApplicationStateTransition struct {
	ID            string           `json:"id"`
	ApplicationID string           `json:"application_id"`
	FromState     ApplicationState `json:"from_state"`
	ToState       ApplicationState `json:"to_state"`
	Reason        string           `json:"reason"`
	CreatedAt     time.Time        `json:"created_at"`
}
