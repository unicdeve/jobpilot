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

// MatchRepository is a ports.MatchRepository backed by SQLite.
type MatchRepository struct {
	db *sql.DB
}

// NewMatchRepository constructs a MatchRepository over db.
func NewMatchRepository(db *sql.DB) *MatchRepository {
	return &MatchRepository{db: db}
}

// Save inserts a new job-match analysis, generating an ID and timestamp if
// unset.
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

// GetByID retrieves a match analysis by ID.
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
		if errors.Is(err, sql.ErrNoRows) {
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

// GetByJobID retrieves the latest match analysis for a job, returning
// (nil, nil) if none exists. Analyzing a job again adds an analysis rather
// than replacing one, so the newest is the one that stands.
func (r *MatchRepository) GetByJobID(ctx context.Context, jobID string) (*domain.JobMatch, error) {
	query := `SELECT
		id, job_id, candidate_id, score, confidence, strengths,
		gaps, concerns, matched_skills, missing_skills,
		rationale, model_provider, model_name, created_at
	FROM job_matches WHERE job_id = ? ORDER BY created_at DESC LIMIT 1`

	row := r.db.QueryRowContext(ctx, query, jobID)

	var m domain.JobMatch
	var strengths, gaps, concerns, matchedSkills, missingSkills string

	err := row.Scan(
		&m.ID, &m.JobID, &m.CandidateID, &m.Score, &m.Confidence, &strengths,
		&gaps, &concerns, &matchedSkills, &missingSkills,
		&m.Rationale, &m.ModelProvider, &m.ModelName, &m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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

// ListByCandidate returns the match analyses for a candidate, most recent
// first, leaving out those of jobs the user ruled out: this is the list of
// analyzed jobs they browse, and a ruled-out job leaves every such list.
// GetByJobID still finds one, for the job's own page.
func (r *MatchRepository) ListByCandidate(ctx context.Context, candidateID string) ([]*domain.JobMatch, error) {
	query := `SELECT
		id, job_id, candidate_id, score, confidence, strengths,
		gaps, concerns, matched_skills, missing_skills,
		rationale, model_provider, model_name, created_at
	FROM job_matches
	WHERE candidate_id = ?
		AND job_id NOT IN (SELECT id FROM jobs WHERE not_interested_at IS NOT NULL)
	ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list matches: %w", err)
	}
	defer func() { _ = rows.Close() }()

	matches := make([]*domain.JobMatch, 0)
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
