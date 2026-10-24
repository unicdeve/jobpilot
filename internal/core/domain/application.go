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
	ApplicationStateInterviewing   ApplicationState = "INTERVIEWING"
	ApplicationStateOffered        ApplicationState = "OFFERED"
	ApplicationStateNoResponse     ApplicationState = "NO_RESPONSE"
	ApplicationStateRejected       ApplicationState = "REJECTED"
	ApplicationStateWithdrawn      ApplicationState = "WITHDRAWN"
	ApplicationStateFailed         ApplicationState = "FAILED"
)

// ApplicationMode is how much of an application JobPilot does for the user
// (ADR-0014). Review is the default.
//
// There were three of these. Autopilot was the third, and it was the wrong
// shape: "watch my saved searches and bring me things worth applying to" is
// a standing instruction that produces applications, not a property of one
// of them. That capability moved out to a Scout, and nothing in JobPilot
// submits to an employer any more — see
// docs/09_decisions/0014-scouts-and-no-unattended-submission.md.
type ApplicationMode string

// Application modes, per ADR-0014. Review is the default.
const (
	ApplicationModeAssist ApplicationMode = "assist"
	ApplicationModeReview ApplicationMode = "review"
)

// IsValid reports whether a mode is one JobPilot actually has.
//
// The column behind this is free TEXT with no constraint, so without a
// check at the boundary any string a caller sent became a mode: a typo
// would have created a fourth one silently, and "autopilot" could come
// back by accident after ADR-0014 removed it.
func (m ApplicationMode) IsValid() bool {
	return m == ApplicationModeAssist || m == ApplicationModeReview
}

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
	// ApplyURL is where the user said the application form is, when the
	// job's own link does not get there — a posting behind a check that
	// refuses JobPilot's browser, say (ADR-0033 §4). A browser run starts
	// here when it is set.
	ApplyURL  string    `json:"apply_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
