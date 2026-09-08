package domain

// QuestionType says whether an ApplicationQuestion may be answered by LLM
// generation when no profile fact or deterministic derivation applies.
// Factual questions never reach the LLM (docs/02_features/07_application_answers.md
// AC-003: unknown factual questions are not guessed).
type QuestionType string

// Question types.
const (
	QuestionTypeFactual   QuestionType = "factual"
	QuestionTypeNarrative QuestionType = "narrative"
)

// AnswerCategory records which of the four resolver categories in
// docs/02_features/07_application_answers.md produced an answer.
type AnswerCategory string

// Answer resolver categories.
const (
	AnswerCategoryProfileFact    AnswerCategory = "profile_fact"
	AnswerCategoryDerived        AnswerCategory = "derived"
	AnswerCategoryGenerated      AnswerCategory = "generated"
	AnswerCategoryNeedsUserInput AnswerCategory = "needs_user_input"
)

// QuestionStatus is whether an ApplicationQuestion currently has an answer.
type QuestionStatus string

// Question statuses.
const (
	QuestionStatusPending  QuestionStatus = "pending"
	QuestionStatusAnswered QuestionStatus = "answered"
)

// QuestionSource says how an ApplicationQuestion entered the system.
type QuestionSource string

// Question sources.
const (
	QuestionSourceManual        QuestionSource = "manual"
	QuestionSourceFormExtracted QuestionSource = "form_extracted"
)

// ApplicationQuestion is a single question on an application (e.g. a form
// field) along with its resolved (or pending) answer.
type ApplicationQuestion struct {
	ID            string         `json:"id"`
	ApplicationID string         `json:"application_id"`
	Label         string         `json:"label"`
	Type          QuestionType   `json:"type"`
	Required      bool           `json:"required"`
	Answer        string         `json:"answer"`
	AnswerSource  string         `json:"answer_source"`
	Category      AnswerCategory `json:"category"`
	Confidence    float64        `json:"confidence"`
	UserEdited    bool           `json:"user_edited"`
	Status        QuestionStatus `json:"status"`
	Source        QuestionSource `json:"source"`
	FieldRef      string         `json:"field_ref,omitempty"`
}
