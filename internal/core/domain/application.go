package domain

import "time"

// ApplicationState is a step in an Application's lifecycle. Not every
// application progresses through every state (docs/02_features/08_applications.md).
type ApplicationState string

// Application lifecycle states, per docs/02_features/08_applications.md.
const (
	ApplicationStateDiscovered   ApplicationState = "DISCOVERED"
	ApplicationStateMatched      ApplicationState = "MATCHED"
	ApplicationStateApproved     ApplicationState = "APPROVED"
	ApplicationStateSubmitted    ApplicationState = "SUBMITTED"
	ApplicationStateInterviewing ApplicationState = "INTERVIEWING"
	ApplicationStateOffered      ApplicationState = "OFFERED"
	ApplicationStateNoResponse   ApplicationState = "NO_RESPONSE"
	ApplicationStateRejected     ApplicationState = "REJECTED"
	ApplicationStateWithdrawn    ApplicationState = "WITHDRAWN"
)

// Application tracks a candidate's progress applying to a specific job,
// distinct from the Job itself.
type Application struct {
	ID                   string           `json:"id"`
	JobID                string           `json:"job_id"`
	CandidateID          string           `json:"candidate_id"`
	ResumeID             string           `json:"resume_id"`
	State                ApplicationState `json:"state"`
	CurrentCoverLetterID string           `json:"current_cover_letter_id"`
	SubmittedAt          *time.Time       `json:"submitted_at,omitempty"`
	// ApplyURL is where the user said the application form is, when the
	// job's own link does not get there. "Open application page" opens it
	// in the user's own browser in place of the job's link (ADR-0034 §2).
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
