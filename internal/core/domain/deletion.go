package domain

// RecordKind names a kind of stored record in the product's own words.
//
// Deletion rules are written in these rather than in table names, so that
// what a deletion reaches is a statement about jobs and applications that
// core can test, and the adapter alone knows which tables hold them.
type RecordKind string

// The kinds of stored record.
const (
	RecordCandidate RecordKind = "candidate"
	// RecordCandidateDetail is any of a candidate's experiences, skills,
	// links and work authorizations: parts of the profile rather than
	// records the user handles one at a time.
	RecordCandidateDetail RecordKind = "candidate_detail"
	RecordResume          RecordKind = "resume"
	RecordJob             RecordKind = "job"
	RecordMatch           RecordKind = "match"
	RecordApplication     RecordKind = "application"
	RecordQuestion        RecordKind = "question"
	RecordCoverLetter     RecordKind = "cover_letter"
	RecordStateTransition RecordKind = "state_transition"
	RecordAgentRun        RecordKind = "agent_run"
	RecordAgentStep       RecordKind = "agent_step"
	RecordScout           RecordKind = "scout"
	RecordSavedSearch     RecordKind = "saved_search"
	RecordCredentialName  RecordKind = "credential_name"
	RecordSettings        RecordKind = "settings"
	// RecordPersonalAnswer is something the candidate said about
	// themselves, for every application or for one (ADR-0022).
	RecordPersonalAnswer RecordKind = "personal_answer"
)

// DeletionEffect is what deleting a referenced record does to a record
// that references it.
type DeletionEffect string

const (
	// DeletionCascade deletes the referencing record with it.
	DeletionCascade DeletionEffect = "delete"
	// DeletionDetach keeps the referencing record and clears its reference.
	DeletionDetach DeletionEffect = "detach"
)

// DeletionRule says what deleting a Referenced record does to a
// Referencing one.
type DeletionRule struct {
	Referencing RecordKind
	Referenced  RecordKind
	Effect      DeletionEffect
}

// DeletionRules is the single statement of what a deletion reaches
// (ADR-0021 §1).
//
// The preview a confirmation shows is computed from it, and the schema's
// foreign keys are tested against it rule for rule, so the two cannot
// disagree about what a deletion takes with it without a test failing. A
// kind that references another must be listed here, or that test fails.
var DeletionRules = []DeletionRule{
	{RecordCandidateDetail, RecordCandidate, DeletionCascade},
	{RecordResume, RecordCandidate, DeletionCascade},
	{RecordMatch, RecordJob, DeletionCascade},
	{RecordMatch, RecordCandidate, DeletionCascade},
	{RecordApplication, RecordJob, DeletionCascade},
	{RecordApplication, RecordCandidate, DeletionCascade},
	// A deleted resume leaves the applications that used it with what was
	// already generated from it (spec D5).
	{RecordApplication, RecordResume, DeletionDetach},
	// An application's current cover letter is one of its own versions,
	// which go with it; the reference is cleared rather than blocking.
	{RecordApplication, RecordCoverLetter, DeletionDetach},
	{RecordQuestion, RecordApplication, DeletionCascade},
	{RecordCoverLetter, RecordApplication, DeletionCascade},
	{RecordStateTransition, RecordApplication, DeletionCascade},
	// A browser run is about one application and goes with it (spec D4).
	{RecordAgentRun, RecordApplication, DeletionCascade},
	// A scout run outlives its scout: it is the record of what it did.
	{RecordAgentRun, RecordScout, DeletionDetach},
	{RecordAgentStep, RecordAgentRun, DeletionCascade},
	{RecordScout, RecordSavedSearch, DeletionCascade},
	// What the candidate said about themselves goes with its owner: the
	// profile, or the one application it was said for (ADR-0022 §1).
	{RecordPersonalAnswer, RecordCandidate, DeletionCascade},
	{RecordPersonalAnswer, RecordApplication, DeletionCascade},
}

// RulesReferencing returns the rules for records that reference kind: what
// deleting a record of that kind does to the others.
func RulesReferencing(kind RecordKind) []DeletionRule {
	var rules []DeletionRule
	for _, rule := range DeletionRules {
		if rule.Referenced == kind {
			rules = append(rules, rule)
		}
	}
	return rules
}

// DeletionTarget names what the user asked to delete.
type DeletionTarget string

// What can be deleted (docs/02_features/15_data_deletion.md). The first
// four name one record by ID; the rest take none.
const (
	DeleteJob         DeletionTarget = "job"
	DeleteApplication DeletionTarget = "application"
	DeleteCoverLetter DeletionTarget = "cover_letter"
	DeleteResume      DeletionTarget = "resume"
	DeleteProfile     DeletionTarget = "profile"
	// ClearActivity deletes every agent run and its steps.
	ClearActivity DeletionTarget = "activity"
	// ClearJobs deletes every job no application uses, except those marked
	// not interested, which each hold a decision the user made (spec D10).
	ClearJobs DeletionTarget = "jobs"
	// DeleteEverything returns the installation to a fresh install (spec D7).
	DeleteEverything DeletionTarget = "everything"
)

// TakesID reports whether the target names one record.
func (t DeletionTarget) TakesID() bool {
	switch t {
	case DeleteJob, DeleteApplication, DeleteCoverLetter, DeleteResume:
		return true
	}
	return false
}

// Valid reports whether t is a known target.
func (t DeletionTarget) Valid() bool {
	switch t {
	case DeleteJob, DeleteApplication, DeleteCoverLetter, DeleteResume,
		DeleteProfile, ClearActivity, ClearJobs, DeleteEverything:
		return true
	}
	return false
}

// DeletionReach is what a deletion takes with it: what the user is shown
// before confirming, and what is then deleted, no more.
type DeletionReach struct {
	Target DeletionTarget `json:"target"`
	ID     string         `json:"id,omitempty"`
	// Deleted counts the records that go, by kind, the target included.
	Deleted map[RecordKind]int `json:"deleted"`
	// Detached counts the records that stay but lose a reference to one
	// that goes: applications whose resume is deleted, say.
	Detached map[RecordKind]int `json:"detached"`
	// SentApplications is how many of the applications that go were sent
	// to an employer. Deleting them withdraws nothing, and the
	// confirmation says so.
	SentApplications int `json:"sent_applications"`
	// KeptNotInterested is how many jobs "clear jobs" leaves because they
	// are marked not interested.
	KeptNotInterested int `json:"kept_not_interested,omitempty"`
	// Outside names what "delete all data" removes beyond the database, in
	// the order it removes them.
	Outside []string `json:"outside,omitempty"`
	// EnvironmentCredentials names the environment variables that supply
	// an API key. JobPilot cannot unset them, so they outlive "delete all
	// data", and the confirmation says so.
	EnvironmentCredentials []string `json:"environment_credentials,omitempty"`
	// Token identifies exactly what this reach covers. A deletion is
	// carried out only with the token of the preview the user confirmed,
	// and refused if what it would reach has changed since (ADR-0021 §4).
	Token string `json:"token"`
}

// DeletionResult is what a deletion did.
type DeletionResult struct {
	Reach DeletionReach `json:"reach"`
	// Notices are what the deletion could not do and the user should know:
	// an API key an environment variable still supplies, say.
	Notices []string `json:"notices,omitempty"`
}
