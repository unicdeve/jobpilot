package domain

import "time"

// CoverLetter is one generated (or user-edited) version of an Application's
// cover letter. Each regeneration creates a new row rather than overwriting
// the previous one, so prior attempts remain part of the application's
// history (docs/02_features/06_cover_letters.md AC-005).
type CoverLetter struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Content       string    `json:"content"`
	ModelProvider string    `json:"model_provider"`
	ModelName     string    `json:"model_name"`
	UserEdited    bool      `json:"user_edited"`
	CreatedAt     time.Time `json:"created_at"`
	// Flags are what the reviewer should look at in Content. They are
	// worked out each time the letter is read and never stored
	// (docs/02_features/19_draft_checks.md D3).
	Flags []DraftFlag `json:"flags,omitempty"`
}
