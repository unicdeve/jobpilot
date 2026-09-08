package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/unicdev/jobpilot/internal/core/domain"
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

// Save inserts a new application question, generating an ID if unset.
func (r *ApplicationQuestionRepository) Save(ctx context.Context, question *domain.ApplicationQuestion) error {
	if question.ID == "" {
		question.ID = uuid.NewString()
	}

	query := `INSERT INTO application_questions (
		id, application_id, label, type, required, answer,
		answer_source, category, confidence, user_edited, status
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		question.ID, question.ApplicationID, question.Label, question.Type, question.Required,
		question.Answer, question.AnswerSource, nullableString(string(question.Category)),
		question.Confidence, question.UserEdited, question.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to save application question: %w", err)
	}

	return nil
}

const applicationQuestionColumns = `
	id, application_id, label, type, required, answer,
	answer_source, category, confidence, user_edited, status`

func scanApplicationQuestion(scanner interface{ Scan(...any) error }) (*domain.ApplicationQuestion, error) {
	var q domain.ApplicationQuestion
	var category sql.NullString
	err := scanner.Scan(
		&q.ID, &q.ApplicationID, &q.Label, &q.Type, &q.Required, &q.Answer,
		&q.AnswerSource, &category, &q.Confidence, &q.UserEdited, &q.Status,
	)
	if err != nil {
		return nil, err
	}
	q.Category = domain.AnswerCategory(category.String)
	return &q, nil
}

// GetByID retrieves an application question by ID.
func (r *ApplicationQuestionRepository) GetByID(ctx context.Context, id string) (*domain.ApplicationQuestion, error) {
	query := `SELECT` + applicationQuestionColumns + ` FROM application_questions WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	q, err := scanApplicationQuestion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("application question not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get application question: %w", err)
	}

	return q, nil
}

// Update persists changes to an application question (typically its
// resolved or user-supplied answer).
func (r *ApplicationQuestionRepository) Update(ctx context.Context, question *domain.ApplicationQuestion) error {
	query := `UPDATE application_questions SET
		label = ?, type = ?, required = ?, answer = ?, answer_source = ?,
		category = ?, confidence = ?, user_edited = ?, status = ?
	WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query,
		question.Label, question.Type, question.Required, question.Answer, question.AnswerSource,
		nullableString(string(question.Category)), question.Confidence, question.UserEdited, question.Status,
		question.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update application question: %w", err)
	}

	return nil
}

// ListByApplication returns every question for an application.
func (r *ApplicationQuestionRepository) ListByApplication(ctx context.Context, applicationID string) ([]*domain.ApplicationQuestion, error) {
	query := `SELECT` + applicationQuestionColumns + ` FROM application_questions WHERE application_id = ?`

	rows, err := r.db.QueryContext(ctx, query, applicationID)
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

// Delete removes an application question by ID.
func (r *ApplicationQuestionRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM application_questions WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete application question: %w", err)
	}
	return nil
}
