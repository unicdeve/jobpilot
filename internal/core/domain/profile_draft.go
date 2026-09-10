package domain

// ProfileDraft is an LLM-proposed set of Candidate fields derived from a
// resume, pending user review before being applied to a Candidate profile.
// Fields the LLM couldn't find in the resume are left as their zero value
// rather than guessed, so callers should only apply non-empty fields over
// whatever the user has already entered.
type ProfileDraft struct {
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Contact   string   `json:"contact"`
	Location  string   `json:"location"`
	Summary   string   `json:"summary"`
	Skills    []string `json:"skills"`
}
