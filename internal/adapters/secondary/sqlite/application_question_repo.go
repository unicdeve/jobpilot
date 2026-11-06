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

// ApplicationQuestionRepository is a ports.ApplicationQuestionRepository
// backed by SQLite.
type ApplicationQuestionRepository struct {
	db *sql.DB
}

// NewApplicationQuestionRepository constructs an
// ApplicationQuestionRepository over db.
func NewApplicationQuestionRepository(db *sql.DB) *ApplicationQuestionRepository {
	return &ApplicationQuestionRepository{db: db}
}

// Save inserts a new application question, generating an ID and timestamp
// if unset.
func (r *ApplicationQuestionRepository) Save(ctx context.Context, question *domain.ApplicationQuestion) error {
	if question.ID == "" {
		question.ID = uuid.NewString()
	}
	if question.CreatedAt.IsZero() {
		question.CreatedAt = time.Now()
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO application_questions (id, application_id, label, answer, created_at) VALUES (?, ?, ?, ?, ?)`,
		question.ID, question.ApplicationID, question.Label, question.Answer, question.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save application question: %w", err)
	}
	return nil
}

const applicationQuestionColumns = ` id, application_id, label, answer, created_at`

func scanApplicationQuestion(scanner interface{ Scan(...any) error }) (*domain.ApplicationQuestion, error) {
	var q domain.ApplicationQuestion
	if err := scanner.Scan(&q.ID, &q.ApplicationID, &q.Label, &q.Answer, &q.CreatedAt); err != nil {
		return nil, err
	}
	return &q, nil
}

// GetByID retrieves an application question by ID.
func (r *ApplicationQuestionRepository) GetByID(ctx context.Context, id string) (*domain.ApplicationQuestion, error) {
	row := r.db.QueryRowContext(ctx, `SELECT`+applicationQuestionColumns+` FROM application_questions WHERE id = ?`, id)
	q, err := scanApplicationQuestion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ports.NotFoundError{Kind: "application question", ID: id}
		}
		return nil, fmt.Errorf("failed to get application question: %w", err)
	}
	return q, nil
}

// Update persists a question's label and answer.
func (r *ApplicationQuestionRepository) Update(ctx context.Context, question *domain.ApplicationQuestion) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE application_questions SET label = ?, answer = ? WHERE id = ?`,
		question.Label, question.Answer, question.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update application question: %w", err)
	}
	return nil
}

// ListByApplication returns every question for an application, oldest
// first.
func (r *ApplicationQuestionRepository) ListByApplication(ctx context.Context, applicationID string) ([]*domain.ApplicationQuestion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT`+applicationQuestionColumns+` FROM application_questions WHERE application_id = ? ORDER BY created_at ASC, rowid ASC`,
		applicationID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list application questions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	questions := make([]*domain.ApplicationQuestion, 0)
	for rows.Next() {
		q, err := scanApplicationQuestion(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan application question: %w", err)
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating application questions: %w", err)
	}
	return questions, nil
}

// Delete removes a question.
func (r *ApplicationQuestionRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM application_questions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete application question: %w", err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return &ports.NotFoundError{Kind: "application question", ID: id}
	}
	return nil
}
