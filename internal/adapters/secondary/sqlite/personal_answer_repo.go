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

// PersonalAnswerRepository is a ports.PersonalAnswerRepository backed by
// SQLite.
type PersonalAnswerRepository struct {
	db *sql.DB
}

// NewPersonalAnswerRepository constructs a PersonalAnswerRepository over db.
func NewPersonalAnswerRepository(db *sql.DB) *PersonalAnswerRepository {
	return &PersonalAnswerRepository{db: db}
}

const personalAnswerColumns = `id, candidate_id, key, question, answer, created_at, updated_at`

func scanPersonalAnswer(row interface{ Scan(...any) error }) (*domain.PersonalAnswer, error) {
	var a domain.PersonalAnswer
	err := row.Scan(&a.ID, &a.CandidateID, &a.Key, &a.Question, &a.Answer, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListByCandidate returns the candidate's answers, oldest first.
func (r *PersonalAnswerRepository) ListByCandidate(ctx context.Context, candidateID string) ([]*domain.PersonalAnswer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+personalAnswerColumns+` FROM personal_answers WHERE candidate_id = ? ORDER BY created_at, rowid`, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list personal answers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	answers := make([]*domain.PersonalAnswer, 0)
	for rows.Next() {
		answer, err := scanPersonalAnswer(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan personal answer: %w", err)
		}
		answers = append(answers, answer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating personal answers: %w", err)
	}
	return answers, nil
}

// GetByID returns one answer.
func (r *PersonalAnswerRepository) GetByID(ctx context.Context, id string) (*domain.PersonalAnswer, error) {
	answer, err := scanPersonalAnswer(r.db.QueryRowContext(ctx,
		`SELECT `+personalAnswerColumns+` FROM personal_answers WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ports.NotFoundError{Kind: "personal answer", ID: id}
		}
		return nil, fmt.Errorf("failed to get personal answer %s: %w", id, err)
	}
	return answer, nil
}

// Save inserts an answer without an ID, and otherwise updates its question
// and answer. Its owner and key never change.
func (r *PersonalAnswerRepository) Save(ctx context.Context, answer *domain.PersonalAnswer) error {
	now := time.Now().UTC()
	if answer.ID == "" {
		answer.ID = uuid.NewString()
		answer.CreatedAt, answer.UpdatedAt = now, now
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO personal_answers (`+personalAnswerColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			answer.ID, answer.CandidateID, answer.Key, answer.Question, answer.Answer, answer.CreatedAt, answer.UpdatedAt)
		if err != nil {
			answer.ID = ""
			return fmt.Errorf("failed to save personal answer: %w", err)
		}
		return nil
	}

	answer.UpdatedAt = now
	result, err := r.db.ExecContext(ctx,
		`UPDATE personal_answers SET question = ?, answer = ?, updated_at = ? WHERE id = ?`,
		answer.Question, answer.Answer, answer.UpdatedAt, answer.ID)
	if err != nil {
		return fmt.Errorf("failed to update personal answer: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("failed to update personal answer: %w", err)
	} else if n == 0 {
		return &ports.NotFoundError{Kind: "personal answer", ID: answer.ID}
	}
	return nil
}

// Delete removes one answer. One already gone is not an error.
func (r *PersonalAnswerRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM personal_answers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete personal answer %s: %w", id, err)
	}
	return nil
}
