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

// ApplicationRepository is a ports.ApplicationRepository backed by SQLite.
type ApplicationRepository struct {
	db *sql.DB
}

// NewApplicationRepository constructs an ApplicationRepository over db.
func NewApplicationRepository(db *sql.DB) *ApplicationRepository {
	return &ApplicationRepository{db: db}
}

// Save inserts a new application, generating an ID and timestamps if unset.
func (r *ApplicationRepository) Save(ctx context.Context, application *domain.Application) error {
	if application.ID == "" {
		application.ID = uuid.NewString()
	}
	if application.CreatedAt.IsZero() {
		application.CreatedAt = time.Now()
	}
	if application.UpdatedAt.IsZero() {
		application.UpdatedAt = time.Now()
	}

	query := `INSERT INTO applications (
		id, job_id, candidate_id, resume_id, mode, state,
		policy_result, approval_status, submitted_at, failure_reason,
		current_cover_letter_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		application.ID, application.JobID, application.CandidateID, nullableString(application.ResumeID),
		application.Mode, application.State, application.PolicyResult, application.ApprovalStatus,
		application.SubmittedAt, application.FailureReason, nullableString(application.CurrentCoverLetterID),
		application.CreatedAt, application.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save application: %w", err)
	}

	return nil
}

const applicationColumns = `
	id, job_id, candidate_id, resume_id, mode, state,
	policy_result, approval_status, submitted_at, failure_reason,
	current_cover_letter_id, created_at, updated_at`

func scanApplication(scanner interface{ Scan(...any) error }) (*domain.Application, error) {
	var a domain.Application
	var resumeID, currentCoverLetterID sql.NullString
	err := scanner.Scan(
		&a.ID, &a.JobID, &a.CandidateID, &resumeID, &a.Mode, &a.State,
		&a.PolicyResult, &a.ApprovalStatus, &a.SubmittedAt, &a.FailureReason,
		&currentCoverLetterID, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.ResumeID = resumeID.String
	a.CurrentCoverLetterID = currentCoverLetterID.String
	return &a, nil
}

// GetByID retrieves an application by ID.
func (r *ApplicationRepository) GetByID(ctx context.Context, id string) (*domain.Application, error) {
	query := `SELECT` + applicationColumns + ` FROM applications WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("application not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get application: %w", err)
	}

	return app, nil
}

// Update persists mutable Application fields other than State, which only
// TransitionState may change.
func (r *ApplicationRepository) Update(ctx context.Context, application *domain.Application) error {
	application.UpdatedAt = time.Now()

	query := `UPDATE applications SET
		resume_id = ?, mode = ?, policy_result = ?, approval_status = ?,
		failure_reason = ?, current_cover_letter_id = ?, updated_at = ?
	WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query,
		nullableString(application.ResumeID), application.Mode, application.PolicyResult, application.ApprovalStatus,
		application.FailureReason, nullableString(application.CurrentCoverLetterID), application.UpdatedAt,
		application.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update application: %w", err)
	}

	return nil
}

// ListByCandidate returns all applications for a candidate, most recently
// updated first.
func (r *ApplicationRepository) ListByCandidate(ctx context.Context, candidateID string) ([]*domain.Application, error) {
	query := `SELECT` + applicationColumns + ` FROM applications WHERE candidate_id = ? ORDER BY updated_at DESC`

	rows, err := r.db.QueryContext(ctx, query, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var apps []*domain.Application
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan application: %w", err)
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating applications: %w", err)
	}

	return apps, nil
}

// Delete removes an application by ID.
func (r *ApplicationRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM applications WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete application: %w", err)
	}
	return nil
}

// TransitionState atomically moves an application from "from" to "to" and
// appends an ApplicationStateTransition record. The UPDATE is conditional
// on the row's current state still being "from"; if zero rows are
// affected (state already changed since the caller read it), the whole
// operation is rolled back and an error is returned.
func (r *ApplicationRepository) TransitionState(ctx context.Context, applicationID string, from, to domain.ApplicationState, reason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	var submittedAt any
	if to == domain.ApplicationStateSubmitted {
		submittedAt = now
	}

	result, err := tx.ExecContext(ctx,
		`UPDATE applications SET state = ?, updated_at = ?, submitted_at = COALESCE(submitted_at, ?)
		 WHERE id = ? AND state = ?`,
		to, now, submittedAt, applicationID, from,
	)
	if err != nil {
		return fmt.Errorf("failed to update application state: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("application %s is not currently in state %s (concurrent update or stale read)", applicationID, from)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO application_state_transitions (id, application_id, from_state, to_state, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), applicationID, from, to, reason, now,
	)
	if err != nil {
		return fmt.Errorf("failed to record state transition: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit state transition: %w", err)
	}

	return nil
}

// ListStateTransitions returns the durable history of an application's
// state changes, oldest first.
func (r *ApplicationRepository) ListStateTransitions(ctx context.Context, applicationID string) ([]*domain.ApplicationStateTransition, error) {
	query := `SELECT id, application_id, from_state, to_state, reason, created_at
		FROM application_state_transitions WHERE application_id = ? ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list state transitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var transitions []*domain.ApplicationStateTransition
	for rows.Next() {
		var t domain.ApplicationStateTransition
		var fromState sql.NullString
		if err := rows.Scan(&t.ID, &t.ApplicationID, &fromState, &t.ToState, &t.Reason, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan state transition: %w", err)
		}
		t.FromState = domain.ApplicationState(fromState.String)
		transitions = append(transitions, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating state transitions: %w", err)
	}

	return transitions, nil
}
