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

// candidateColumns is the candidates row, in the order scanCandidate reads
// it. One list so the three read paths cannot drift apart.
const candidateColumns = `id, first_name, last_name, email, phone,
	location_city, location_region, location_country,
	notice_period_days, earliest_start_date,
	summary, work_preferences, application_preferences,
	skills, experiences, education, certifications, created_at, updated_at`

// Save inserts a new candidate, generating an ID and timestamps if unset.
//
// The candidate's links and work authorizations are written with it, in one
// transaction: a profile that is half-saved would answer a form with half a
// person.
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

	query := `INSERT INTO candidates (` + candidateColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	return r.inTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, query,
			candidate.ID, candidate.FirstName, candidate.LastName, candidate.Email, candidate.Phone,
			candidate.LocationCity, candidate.LocationRegion, candidate.LocationCountry,
			candidate.NoticePeriodDays, candidate.EarliestStartDate,
			candidate.Summary, candidate.WorkPreferences, candidate.ApplicationPreferences,
			candidate.Skills, candidate.Experiences, candidate.Education, candidate.Certifications,
			candidate.CreatedAt, candidate.UpdatedAt,
		); err != nil {
			return fmt.Errorf("failed to save candidate: %w", err)
		}
		return r.replaceChildren(ctx, tx, candidate)
	})
}

// GetByID retrieves a candidate by ID, with their links and work
// authorizations.
func (r *CandidateRepository) GetByID(ctx context.Context, id string) (*domain.Candidate, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+candidateColumns+` FROM candidates WHERE id = ?`, id)

	candidate, err := scanCandidate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("candidate not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get candidate: %w", err)
	}
	if err := r.loadChildren(ctx, candidate); err != nil {
		return nil, err
	}

	return candidate, nil
}

// Update overwrites an existing candidate's fields, links and work
// authorizations.
func (r *CandidateRepository) Update(ctx context.Context, candidate *domain.Candidate) error {
	candidate.UpdatedAt = time.Now()

	query := `UPDATE candidates SET
		first_name = ?, last_name = ?, email = ?, phone = ?,
		location_city = ?, location_region = ?, location_country = ?,
		notice_period_days = ?, earliest_start_date = ?,
		summary = ?, work_preferences = ?, application_preferences = ?,
		skills = ?, experiences = ?, education = ?, certifications = ?, updated_at = ?
	WHERE id = ?`

	return r.inTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, query,
			candidate.FirstName, candidate.LastName, candidate.Email, candidate.Phone,
			candidate.LocationCity, candidate.LocationRegion, candidate.LocationCountry,
			candidate.NoticePeriodDays, candidate.EarliestStartDate,
			candidate.Summary, candidate.WorkPreferences, candidate.ApplicationPreferences,
			candidate.Skills, candidate.Experiences, candidate.Education, candidate.Certifications,
			candidate.UpdatedAt, candidate.ID,
		); err != nil {
			return fmt.Errorf("failed to update candidate: %w", err)
		}
		return r.replaceChildren(ctx, tx, candidate)
	})
}

// Delete removes a candidate by ID, and their links and work
// authorizations with them.
//
// The rows are deleted explicitly rather than left to ON DELETE CASCADE,
// because SQLite enforces foreign keys only when the connection has asked
// it to and a silently orphaned row is worse than a redundant statement.
func (r *CandidateRepository) Delete(ctx context.Context, id string) error {
	return r.inTransaction(ctx, func(tx *sql.Tx) error {
		for _, table := range []string{"candidate_links", "candidate_work_authorizations"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE candidate_id = ?`, id); err != nil {
				return fmt.Errorf("failed to delete candidate's %s: %w", table, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM candidates WHERE id = ?`, id); err != nil {
			return fmt.Errorf("failed to delete candidate: %w", err)
		}
		return nil
	})
}

// GetFirstCandidate returns the first candidate profile in the database.
// JobPilot currently manages a single local candidate profile per
// installation, so this is the standard way to fetch "the" profile.
func (r *CandidateRepository) GetFirstCandidate(ctx context.Context) (*domain.Candidate, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+candidateColumns+` FROM candidates LIMIT 1`)

	candidate, err := scanCandidate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no candidate profile found")
		}
		return nil, fmt.Errorf("failed to get first candidate: %w", err)
	}
	if err := r.loadChildren(ctx, candidate); err != nil {
		return nil, err
	}

	return candidate, nil
}

// scanCandidate reads one candidates row in candidateColumns order.
func scanCandidate(row *sql.Row) (*domain.Candidate, error) {
	var c domain.Candidate
	err := row.Scan(
		&c.ID, &c.FirstName, &c.LastName, &c.Email, &c.Phone,
		&c.LocationCity, &c.LocationRegion, &c.LocationCountry,
		&c.NoticePeriodDays, &c.EarliestStartDate,
		&c.Summary, &c.WorkPreferences, &c.ApplicationPreferences,
		&c.Skills, &c.Experiences, &c.Education, &c.Certifications,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// loadChildren fills in the candidate's links and work authorizations.
func (r *CandidateRepository) loadChildren(ctx context.Context, candidate *domain.Candidate) error {
	links, err := r.listLinks(ctx, candidate.ID)
	if err != nil {
		return err
	}
	authorizations, err := r.listWorkAuthorizations(ctx, candidate.ID)
	if err != nil {
		return err
	}
	candidate.Links = links
	candidate.WorkAuthorizations = authorizations
	return nil
}

func (r *CandidateRepository) listLinks(ctx context.Context, candidateID string) ([]domain.CandidateLink, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, candidate_id, kind, url, created_at, updated_at
		FROM candidate_links WHERE candidate_id = ? ORDER BY kind`, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list candidate links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var links []domain.CandidateLink
	for rows.Next() {
		var link domain.CandidateLink
		if err := rows.Scan(&link.ID, &link.CandidateID, &link.Kind, &link.URL,
			&link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan candidate link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read candidate links: %w", err)
	}
	return links, nil
}

func (r *CandidateRepository) listWorkAuthorizations(ctx context.Context, candidateID string) ([]domain.WorkAuthorization, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, candidate_id, country, is_authorized,
		requires_sponsorship, created_at, updated_at
		FROM candidate_work_authorizations WHERE candidate_id = ? ORDER BY country`, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list work authorizations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var authorizations []domain.WorkAuthorization
	for rows.Next() {
		var a domain.WorkAuthorization
		if err := rows.Scan(&a.ID, &a.CandidateID, &a.Country, &a.IsAuthorized,
			&a.RequiresSponsorship, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan work authorization: %w", err)
		}
		authorizations = append(authorizations, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read work authorizations: %w", err)
	}
	return authorizations, nil
}

// replaceChildren rewrites the candidate's links and work authorizations to
// match what is on the struct.
//
// Delete-then-insert rather than a diff: the sets are a handful of rows the
// user edits as a whole in one form, and a diff would be more code with
// more ways to be subtly wrong for no behaviour anyone could observe.
func (r *CandidateRepository) replaceChildren(ctx context.Context, tx *sql.Tx, candidate *domain.Candidate) error {
	now := time.Now()

	if _, err := tx.ExecContext(ctx, `DELETE FROM candidate_links WHERE candidate_id = ?`, candidate.ID); err != nil {
		return fmt.Errorf("failed to clear candidate links: %w", err)
	}
	for i := range candidate.Links {
		link := &candidate.Links[i]
		if link.ID == "" {
			link.ID = uuid.NewString()
		}
		link.CandidateID = candidate.ID
		if link.CreatedAt.IsZero() {
			link.CreatedAt = now
		}
		link.UpdatedAt = now

		if _, err := tx.ExecContext(ctx, `INSERT INTO candidate_links
			(id, candidate_id, kind, url, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			link.ID, link.CandidateID, link.Kind, link.URL, link.CreatedAt, link.UpdatedAt); err != nil {
			return fmt.Errorf("failed to save candidate link %q: %w", link.Kind, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM candidate_work_authorizations WHERE candidate_id = ?`,
		candidate.ID); err != nil {
		return fmt.Errorf("failed to clear work authorizations: %w", err)
	}
	for i := range candidate.WorkAuthorizations {
		auth := &candidate.WorkAuthorizations[i]
		if auth.ID == "" {
			auth.ID = uuid.NewString()
		}
		auth.CandidateID = candidate.ID
		if auth.CreatedAt.IsZero() {
			auth.CreatedAt = now
		}
		auth.UpdatedAt = now

		if _, err := tx.ExecContext(ctx, `INSERT INTO candidate_work_authorizations
			(id, candidate_id, country, is_authorized, requires_sponsorship, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			auth.ID, auth.CandidateID, auth.Country, auth.IsAuthorized, auth.RequiresSponsorship,
			auth.CreatedAt, auth.UpdatedAt); err != nil {
			return fmt.Errorf("failed to save work authorization for %q: %w", auth.Country, err)
		}
	}

	return nil
}

func (r *CandidateRepository) inTransaction(ctx context.Context, do func(tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	if err := do(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}
