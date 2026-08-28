package domain

import "time"

type Resume struct {
	ID               string    `json:"id"`
	CandidateID      string    `json:"candidate_id"`
	Name             string    `json:"name"`
	OriginalFilePath string    `json:"original_file_path"`
	MarkdownContent  string    `json:"markdown_content"`
	ParsedMetadata   string    `json:"parsed_metadata"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
