package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// CoverLetterRepository is a ports.CoverLetterRepository backed by SQLite.
type CoverLetterRepository struct {
	db *sql.DB
}

// NewCoverLetterRepository constructs a CoverLetterRepository over db.
func NewCoverLetterRepository(db *sql.DB) *CoverLetterRepository {
	return &CoverLetterRepository{db: db}
}

// Save inserts a new cover letter version, generating an ID and timestamp
// if unset.
func (r *CoverLetterRepository) Save(ctx context.Context, coverLetter *domain.CoverLetter) error {
	if coverLetter.ID == "" {
		coverLetter.ID = uuid.NewString()
	}
	if coverLetter.CreatedAt.IsZero() {
		coverLetter.CreatedAt = time.Now()
	}

	query := `INSERT INTO cover_letters (
		id, application_id, content, model_provider, model_name, user_edited, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		coverLetter.ID, coverLetter.ApplicationID, coverLetter.Content,
		coverLetter.ModelProvider, coverLetter.ModelName, coverLetter.UserEdited, coverLetter.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save cover letter: %w", err)
	}

	return nil
}

const coverLetterColumns = `
	id, application_id, content, model_provider, model_name, user_edited, created_at`

func scanCoverLetter(scanner interface{ Scan(...any) error }) (*domain.CoverLetter, error) {
	var c domain.CoverLetter
	if err := scanner.Scan(&c.ID, &c.ApplicationID, &c.Content, &c.ModelProvider, &c.ModelName, &c.UserEdited, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetByID retrieves a cover letter version by ID.
func (r *CoverLetterRepository) GetByID(ctx context.Context, id string) (*domain.CoverLetter, error) {
	query := `SELECT` + coverLetterColumns + ` FROM cover_letters WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	c, err := scanCoverLetter(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ports.NotFoundError{Kind: "cover letter", ID: id}
		}
		return nil, fmt.Errorf("failed to get cover letter: %w", err)
	}

	return c, nil
}

// Update edits an existing cover letter version in place (a user editing
// generated content), rather than creating a new version.
func (r *CoverLetterRepository) Update(ctx context.Context, coverLetter *domain.CoverLetter) error {
	query := `UPDATE cover_letters SET content = ?, user_edited = ? WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, coverLetter.Content, coverLetter.UserEdited, coverLetter.ID)
	if err != nil {
		return fmt.Errorf("failed to update cover letter: %w", err)
	}

	return nil
}

// ListByApplication returns every cover letter version for an application,
// oldest first, so callers can see the full regeneration history.
func (r *CoverLetterRepository) ListByApplication(ctx context.Context, applicationID string) ([]*domain.CoverLetter, error) {
	query := `SELECT` + coverLetterColumns + ` FROM cover_letters WHERE application_id = ? ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list cover letters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	letters := make([]*domain.CoverLetter, 0)
	for rows.Next() {
		c, err := scanCoverLetter(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cover letter: %w", err)
		}
		letters = append(letters, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating cover letters: %w", err)
	}

	return letters, nil
}
