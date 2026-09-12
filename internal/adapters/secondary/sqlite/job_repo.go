package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// JobRepository is a ports.JobRepository backed by SQLite.
type JobRepository struct {
	db *sql.DB
}

// NewJobRepository constructs a JobRepository over db.
func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{db: db}
}

// Save inserts a new job posting, generating an ID and timestamps if unset.
func (r *JobRepository) Save(ctx context.Context, job *domain.Job) error {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.DiscoveredAt.IsZero() {
		job.DiscoveredAt = time.Now()
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = time.Now()
	}

	query := `INSERT INTO jobs (
		id, canonical_url, source_id, source_job_id, title, company_name,
		company_url, description_markdown, locations, remote_type,
		employment_type, salary_min, salary_max, salary_currency,
		salary_period, experience_min, experience_max, skills,
		posted_at, expires_at, discovered_at, updated_at, raw_source_reference
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		job.ID, job.CanonicalURL, job.SourceID, job.SourceJobID, job.Title, job.CompanyName,
		job.CompanyURL, job.DescriptionMarkdown, joinStrings(job.Locations), job.RemoteType,
		job.EmploymentType, job.SalaryMin, job.SalaryMax, job.SalaryCurrency,
		job.SalaryPeriod, job.ExperienceMin, job.ExperienceMax, joinStrings(job.Skills),
		job.PostedAt, job.ExpiresAt, job.DiscoveredAt, job.UpdatedAt, job.RawSourceReference,
	)
	if err != nil {
		return fmt.Errorf("failed to save job: %w", err)
	}

	return nil
}

// GetByID retrieves a job by ID.
func (r *JobRepository) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	query := `SELECT
		id, canonical_url, source_id, source_job_id, title, company_name,
		company_url, description_markdown, locations, remote_type,
		employment_type, salary_min, salary_max, salary_currency,
		salary_period, experience_min, experience_max, skills,
		posted_at, expires_at, discovered_at, updated_at, raw_source_reference
	FROM jobs WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)

	var j domain.Job
	var locations, skills string

	err := row.Scan(
		&j.ID, &j.CanonicalURL, &j.SourceID, &j.SourceJobID, &j.Title, &j.CompanyName,
		&j.CompanyURL, &j.DescriptionMarkdown, &locations, &j.RemoteType,
		&j.EmploymentType, &j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
		&j.SalaryPeriod, &j.ExperienceMin, &j.ExperienceMax, &skills,
		&j.PostedAt, &j.ExpiresAt, &j.DiscoveredAt, &j.UpdatedAt, &j.RawSourceReference,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("job not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	j.Locations = splitStrings(locations)
	j.Skills = splitStrings(skills)

	return &j, nil
}

// GetByCanonicalURL retrieves a job by its canonical URL, returning (nil,
// nil) if no such job exists.
func (r *JobRepository) GetByCanonicalURL(ctx context.Context, url string) (*domain.Job, error) {
	query := `SELECT
		id, canonical_url, source_id, source_job_id, title, company_name,
		company_url, description_markdown, locations, remote_type,
		employment_type, salary_min, salary_max, salary_currency,
		salary_period, experience_min, experience_max, skills,
		posted_at, expires_at, discovered_at, updated_at, raw_source_reference
	FROM jobs WHERE canonical_url = ?`

	row := r.db.QueryRowContext(ctx, query, url)

	var j domain.Job
	var locations, skills string

	err := row.Scan(
		&j.ID, &j.CanonicalURL, &j.SourceID, &j.SourceJobID, &j.Title, &j.CompanyName,
		&j.CompanyURL, &j.DescriptionMarkdown, &locations, &j.RemoteType,
		&j.EmploymentType, &j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
		&j.SalaryPeriod, &j.ExperienceMin, &j.ExperienceMax, &skills,
		&j.PostedAt, &j.ExpiresAt, &j.DiscoveredAt, &j.UpdatedAt, &j.RawSourceReference,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job by URL: %w", err)
	}

	j.Locations = splitStrings(locations)
	j.Skills = splitStrings(skills)

	return &j, nil
}

// keywordFilter builds the WHERE clause and arguments that restrict a job
// query to rows matching keywords, or empty values when keywords is blank.
//
// The match is a case-insensitive substring over title and company name.
// That is deliberately the same naive semantic the discovery connectors
// apply to their own results (see JobDiscoveryService) rather than a new,
// better matching rule: browsing stored jobs and searching sources should
// not disagree about what "golang" matches.
func keywordFilter(keywords string) (string, []any) {
	keywords = strings.TrimSpace(keywords)
	if keywords == "" {
		return "", nil
	}

	pattern := "%" + strings.ToLower(keywords) + "%"
	return " WHERE (LOWER(title) LIKE ? OR LOWER(company_name) LIKE ?)", []any{pattern, pattern}
}

// Count returns how many stored jobs match keywords. Callers paginating
// through List use it to tell whether more pages remain.
func (r *JobRepository) Count(ctx context.Context, keywords string) (int, error) {
	where, args := keywordFilter(keywords)

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs"+where, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count jobs: %w", err)
	}
	return total, nil
}

// List returns stored jobs ordered by discovery time, most recent first,
// optionally filtered by keywords and paginated by limit/offset. It reads
// only local history and never contacts a job source, so it works offline.
func (r *JobRepository) List(ctx context.Context, keywords string, limit, offset int) ([]*domain.Job, error) {
	where, args := keywordFilter(keywords)

	query := `SELECT
		id, canonical_url, source_id, source_job_id, title, company_name,
		company_url, description_markdown, locations, remote_type,
		employment_type, salary_min, salary_max, salary_currency,
		salary_period, experience_min, experience_max, skills,
		posted_at, expires_at, discovered_at, updated_at, raw_source_reference
	FROM jobs` + where + ` ORDER BY discovered_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, append(args, limit, offset)...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]*domain.Job, 0)
	for rows.Next() {
		var j domain.Job
		var locations, skills string

		err := rows.Scan(
			&j.ID, &j.CanonicalURL, &j.SourceID, &j.SourceJobID, &j.Title, &j.CompanyName,
			&j.CompanyURL, &j.DescriptionMarkdown, &locations, &j.RemoteType,
			&j.EmploymentType, &j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
			&j.SalaryPeriod, &j.ExperienceMin, &j.ExperienceMax, &skills,
			&j.PostedAt, &j.ExpiresAt, &j.DiscoveredAt, &j.UpdatedAt, &j.RawSourceReference,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}

		j.Locations = splitStrings(locations)
		j.Skills = splitStrings(skills)
		jobs = append(jobs, &j)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}

	return jobs, nil
}

// Delete removes a job by ID.
func (r *JobRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM jobs WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}
	return nil
}
