package domain

import "time"

// CoverLetter is one version of the cover letter the candidate wrote for an
// Application. Saving a new version adds a row rather than overwriting the
// previous one, so earlier versions remain part of the application's
// history (docs/02_features/06_cover_letters.md AC-005).
type CoverLetter struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Content       string    `json:"content"`
	CreatedAt     time.Time `json:"created_at"`
	// Flags are what the candidate may want to look at in Content. They
	// are worked out each time the letter is read and never stored
	// (docs/02_features/19_draft_checks.md D3).
	Flags []DraftFlag `json:"flags,omitempty"`
}
