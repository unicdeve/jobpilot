package domain

import "time"

// JobMatch represents the result of evaluating a job against a candidate profile.
type JobMatch struct {
	ID            string    `json:"id"`
	JobID         string    `json:"job_id"`
	CandidateID   string    `json:"candidate_id"`
	Score         int       `json:"score"`
	Confidence    float64   `json:"confidence"`
	Strengths     []string  `json:"strengths"`
	Gaps          []string  `json:"gaps"`
	Concerns      []string  `json:"concerns"`
	MatchedSkills []string  `json:"matched_skills"`
	MissingSkills []string  `json:"missing_skills"`
	Rationale     string    `json:"rationale"`
	ModelProvider string    `json:"model_provider"`
	ModelName     string    `json:"model_name"`
	CreatedAt     time.Time `json:"created_at"`
}
