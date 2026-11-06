package domain

import "time"

// ApplicationQuestion is a question the candidate noted on an application,
// with the answer they wrote to it (ADR-0034 §4). JobPilot never writes the
// answer; it may only suggest one from a deterministic source.
type ApplicationQuestion struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Label         string    `json:"label"`
	Answer        string    `json:"answer"`
	CreatedAt     time.Time `json:"created_at"`
	// Suggestion is what the candidate's own material says to this
	// question, when anything does. It is worked out each time the
	// question is read and never stored, so it always reflects the profile
	// as it is now.
	Suggestion *AnswerSuggestion `json:"suggestion,omitempty"`
	// Flags are what the candidate may want to look at in Answer, worked
	// out each time it is read and never stored
	// (docs/02_features/19_draft_checks.md D3).
	Flags []DraftFlag `json:"flags,omitempty"`
}

// SuggestionSource says where a suggested answer came from.
type SuggestionSource string

// Suggestion sources, in the order the resolver tries them
// (docs/02_features/07_application_answers.md).
const (
	// SuggestionFromProfile is a fact the profile states: a name, an email
	// address, a link, a right to work in a named country.
	SuggestionFromProfile SuggestionSource = "profile"
	// SuggestionDerived is worked out from the profile: years of
	// experience, a remote-work preference.
	SuggestionDerived SuggestionSource = "derived"
	// SuggestionKeptAnswer is an answer the candidate wrote to a similar
	// question and kept.
	SuggestionKeptAnswer SuggestionSource = "kept_answer"
)

// AnswerSuggestion is an answer the candidate's own material gives to a
// question, offered for them to use, edit or ignore.
type AnswerSuggestion struct {
	Answer string           `json:"answer"`
	Source SuggestionSource `json:"source"`
	// Field names what answered: the profile field ("candidate.email") for
	// a fact or a derivation, the kept question for a kept answer.
	Field string `json:"field"`
}
