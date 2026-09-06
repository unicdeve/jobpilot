// Package sqlite implements the core/ports repository interfaces on top of
// a local SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// CandidateRepository is a ports.CandidateRepository backed by SQLite.
type CandidateRepository struct {
	db *sql.DB
}

// NewCandidateRepository constructs a CandidateRepository over db.
func NewCandidateRepository(db *sql.DB) *CandidateRepository {
	return &CandidateRepository{db: db}
}

// Save inserts a new candidate, generating an ID and timestamps if unset.
func (r *CandidateRepository) Save(ctx context.Context, candidate *domain.Candidate) error {
	if candidate.ID == "" {
		candidate.ID = uuid.NewString()
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	if candidate.UpdatedAt.IsZero() {
		candidate.UpdatedAt = time.Now()
	}

	query := `INSERT INTO candidates (
		id, name, contact, location, summary, work_preferences,
		salary_preferences, work_authorization, application_preferences, skills, experiences,
		education, certifications, links, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		candidate.ID, candidate.Name, candidate.Contact, candidate.Location, candidate.Summary,
		candidate.WorkPreferences, candidate.SalaryPreferences, candidate.WorkAuthorization,
		candidate.ApplicationPreferences, candidate.Skills, candidate.Experiences, candidate.Education,
		candidate.Certifications, candidate.Links, candidate.CreatedAt, candidate.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save candidate: %w", err)
	}

	return nil
}

// GetByID retrieves a candidate by ID.
func (r *CandidateRepository) GetByID(ctx context.Context, id string) (*domain.Candidate, error) {
	query := `SELECT
		id, name, contact, location, summary, work_preferences,
		salary_preferences, work_authorization, application_preferences, skills, experiences,
		education, certifications, links, created_at, updated_at
	FROM candidates WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)

	var c domain.Candidate
	err := row.Scan(
		&c.ID, &c.Name, &c.Contact, &c.Location, &c.Summary,
		&c.WorkPreferences, &c.SalaryPreferences, &c.WorkAuthorization,
		&c.ApplicationPreferences, &c.Skills, &c.Experiences, &c.Education,
		&c.Certifications, &c.Links, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("candidate not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get candidate: %w", err)
	}

	return &c, nil
}

// Update overwrites an existing candidate's fields.
func (r *CandidateRepository) Update(ctx context.Context, candidate *domain.Candidate) error {
	candidate.UpdatedAt = time.Now()

	query := `UPDATE candidates SET
		name = ?, contact = ?, location = ?, summary = ?, work_preferences = ?,
		salary_preferences = ?, work_authorization = ?, application_preferences = ?, skills = ?, experiences = ?,
		education = ?, certifications = ?, links = ?, updated_at = ?
	WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query,
		candidate.Name, candidate.Contact, candidate.Location, candidate.Summary,
		candidate.WorkPreferences, candidate.SalaryPreferences, candidate.WorkAuthorization,
		candidate.ApplicationPreferences, candidate.Skills, candidate.Experiences, candidate.Education,
		candidate.Certifications, candidate.Links, candidate.UpdatedAt, candidate.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update candidate: %w", err)
	}

	return nil
}

// Delete removes a candidate by ID.
func (r *CandidateRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM candidates WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete candidate: %w", err)
	}

	return nil
}

// GetFirstCandidate returns the first candidate profile in the database.
// JobPilot currently manages a single local candidate profile per
// installation, so this is the standard way to fetch "the" profile.
func (r *CandidateRepository) GetFirstCandidate(ctx context.Context) (*domain.Candidate, error) {
	query := `SELECT
		id, name, contact, location, summary, work_preferences,
		salary_preferences, work_authorization, application_preferences, skills, experiences,
		education, certifications, links, created_at, updated_at
	FROM candidates LIMIT 1`

	row := r.db.QueryRowContext(ctx, query)

	var c domain.Candidate
	err := row.Scan(
		&c.ID, &c.Name, &c.Contact, &c.Location, &c.Summary,
		&c.WorkPreferences, &c.SalaryPreferences, &c.WorkAuthorization,
		&c.ApplicationPreferences, &c.Skills, &c.Experiences, &c.Education,
		&c.Certifications, &c.Links, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no candidate profile found")
		}
		return nil, fmt.Errorf("failed to get first candidate: %w", err)
	}

	return &c, nil
}
