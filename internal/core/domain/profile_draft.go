package domain

// ProfileDraft is an LLM-proposed Summary and Skills list derived from a
// resume, pending user review before being applied to a Candidate profile.
type ProfileDraft struct {
	Summary string   `json:"summary"`
	Skills  []string `json:"skills"`
}
