// Package domain contains the core business entities shared across services,
// independent of any storage, transport, or LLM implementation detail.
package domain

import "time"

// Candidate represents a job seeker's profile: identity, preferences, and
// career history used to evaluate and apply to jobs.
type Candidate struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Contact                string    `json:"contact"`
	Location               string    `json:"location"`
	Summary                string    `json:"summary"`
	WorkPreferences        string    `json:"work_preferences"`
	SalaryPreferences      string    `json:"salary_preferences"`
	WorkAuthorization      string    `json:"work_authorization"`
	ApplicationPreferences string    `json:"application_preferences"`
	Skills                 string    `json:"skills"`
	Experiences            string    `json:"experiences"`
	Education              string    `json:"education"`
	Certifications         string    `json:"certifications"`
	Links                  string    `json:"links"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
