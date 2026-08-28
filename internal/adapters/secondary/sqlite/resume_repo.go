package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/job-agent/internal/core/domain"
)

type ResumeRepository struct {
	db *sql.DB
}

func NewResumeRepository(db *sql.DB) *ResumeRepository {
	return &ResumeRepository{db: db}
}

func (r *ResumeRepository) Save(ctx context.Context, resume *domain.Resume) error {
	if resume.ID == "" {
		resume.ID = uuid.NewString()
	}
	if resume.CreatedAt.IsZero() {
		resume.CreatedAt = time.Now()
	}
	if resume.UpdatedAt.IsZero() {
		resume.UpdatedAt = time.Now()
	}

	query := `INSERT INTO resumes (
		id, candidate_id, name, original_file_path, markdown_content, parsed_metadata, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		resume.ID, resume.CandidateID, resume.Name, resume.OriginalFilePath,
		resume.MarkdownContent, resume.ParsedMetadata, resume.CreatedAt, resume.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save resume: %w", err)
	}

	return nil
}

func (r *ResumeRepository) GetByID(ctx context.Context, id string) (*domain.Resume, error) {
	query := `SELECT
		id, candidate_id, name, original_file_path, markdown_content, parsed_metadata, created_at, updated_at
	FROM resumes WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)

	var res domain.Resume
	err := row.Scan(
		&res.ID, &res.CandidateID, &res.Name, &res.OriginalFilePath,
		&res.MarkdownContent, &res.ParsedMetadata, &res.CreatedAt, &res.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("resume not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get resume: %w", err)
	}

	return &res, nil
}

func (r *ResumeRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM resumes WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete resume: %w", err)
	}

	return nil
}
