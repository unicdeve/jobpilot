package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/job-agent/internal/core/domain"
)

type MatchRepository struct {
	db *sql.DB
}

func NewMatchRepository(db *sql.DB) *MatchRepository {
	return &MatchRepository{db: db}
}

func (r *MatchRepository) Save(ctx context.Context, match *domain.JobMatch) error {
	if match.ID == "" {
		match.ID = uuid.NewString()
	}
	if match.CreatedAt.IsZero() {
		match.CreatedAt = time.Now()
	}

	query := `INSERT INTO job_matches (
		id, job_id, candidate_id, score, confidence, strengths,
		gaps, concerns, matched_skills, missing_skills,
		rationale, model_provider, model_name, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		match.ID, match.JobID, match.CandidateID, match.Score, match.Confidence,
		joinStrings(match.Strengths), joinStrings(match.Gaps), joinStrings(match.Concerns),
		joinStrings(match.MatchedSkills), joinStrings(match.MissingSkills),
		match.Rationale, match.ModelProvider, match.ModelName, match.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save match: %w", err)
	}

	return nil
}

func (r *MatchRepository) GetByID(ctx context.Context, id string) (*domain.JobMatch, error) {
	query := `SELECT
		id, job_id, candidate_id, score, confidence, strengths,
		gaps, concerns, matched_skills, missing_skills,
		rationale, model_provider, model_name, created_at
	FROM job_matches WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)

	var m domain.JobMatch
	var strengths, gaps, concerns, matchedSkills, missingSkills string

	err := row.Scan(
		&m.ID, &m.JobID, &m.CandidateID, &m.Score, &m.Confidence, &strengths,
		&gaps, &concerns, &matchedSkills, &missingSkills,
		&m.Rationale, &m.ModelProvider, &m.ModelName, &m.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("match not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get match: %w", err)
	}

	m.Strengths = splitStrings(strengths)
	m.Gaps = splitStrings(gaps)
	m.Concerns = splitStrings(concerns)
	m.MatchedSkills = splitStrings(matchedSkills)
	m.MissingSkills = splitStrings(missingSkills)

	return &m, nil
}

func (r *MatchRepository) GetByJobID(ctx context.Context, jobID string) (*domain.JobMatch, error) {
	query := `SELECT
		id, job_id, candidate_id, score, confidence, strengths,
		gaps, concerns, matched_skills, missing_skills,
		rationale, model_provider, model_name, created_at
	FROM job_matches WHERE job_id = ?`

	row := r.db.QueryRowContext(ctx, query, jobID)

	var m domain.JobMatch
	var strengths, gaps, concerns, matchedSkills, missingSkills string

	err := row.Scan(
		&m.ID, &m.JobID, &m.CandidateID, &m.Score, &m.Confidence, &strengths,
		&gaps, &concerns, &matchedSkills, &missingSkills,
		&m.Rationale, &m.ModelProvider, &m.ModelName, &m.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get match by job ID: %w", err)
	}

	m.Strengths = splitStrings(strengths)
	m.Gaps = splitStrings(gaps)
	m.Concerns = splitStrings(concerns)
	m.MatchedSkills = splitStrings(matchedSkills)
	m.MissingSkills = splitStrings(missingSkills)

	return &m, nil
}

func (r *MatchRepository) ListByCandidate(ctx context.Context, candidateID string) ([]*domain.JobMatch, error) {
	query := `SELECT
		id, job_id, candidate_id, score, confidence, strengths,
		gaps, concerns, matched_skills, missing_skills,
		rationale, model_provider, model_name, created_at
	FROM job_matches WHERE candidate_id = ? ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list matches: %w", err)
	}
	defer rows.Close()

	var matches []*domain.JobMatch
	for rows.Next() {
		var m domain.JobMatch
		var strengths, gaps, concerns, matchedSkills, missingSkills string

		err := rows.Scan(
			&m.ID, &m.JobID, &m.CandidateID, &m.Score, &m.Confidence, &strengths,
			&gaps, &concerns, &matchedSkills, &missingSkills,
			&m.Rationale, &m.ModelProvider, &m.ModelName, &m.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan match: %w", err)
		}

		m.Strengths = splitStrings(strengths)
		m.Gaps = splitStrings(gaps)
		m.Concerns = splitStrings(concerns)
		m.MatchedSkills = splitStrings(matchedSkills)
		m.MissingSkills = splitStrings(missingSkills)
		matches = append(matches, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating matches: %w", err)
	}

	return matches, nil
}

func (r *MatchRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM job_matches WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete match: %w", err)
	}
	return nil
}
