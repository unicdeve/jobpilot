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

// jobColumns are the columns scanJob reads, in its order.
const jobColumns = `id, canonical_url, source_id, source_job_id, title, company_name,
	company_url, description_markdown, locations, remote_type,
	employment_type, salary_min, salary_max, salary_currency,
	salary_period, experience_min, experience_max, skills,
	posted_at, expires_at, discovered_at, updated_at, raw_source_reference,
	not_interested_at`

// rowScanner is what *sql.Row and *sql.Rows have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanJob reads a job selected with jobColumns.
func scanJob(row rowScanner) (*domain.Job, error) {
	var j domain.Job
	var locations, skills string
	var notInterested sql.NullTime
	err := row.Scan(
		&j.ID, &j.CanonicalURL, &j.SourceID, &j.SourceJobID, &j.Title, &j.CompanyName,
		&j.CompanyURL, &j.DescriptionMarkdown, &locations, &j.RemoteType,
		&j.EmploymentType, &j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
		&j.SalaryPeriod, &j.ExperienceMin, &j.ExperienceMax, &skills,
		&j.PostedAt, &j.ExpiresAt, &j.DiscoveredAt, &j.UpdatedAt, &j.RawSourceReference,
		&notInterested,
	)
	if err != nil {
		return nil, err
	}
	j.Locations = splitStrings(locations)
	j.Skills = splitStrings(skills)
	if notInterested.Valid {
		at := notInterested.Time
		j.NotInterestedAt = &at
	}
	return &j, nil
}

// GetByID retrieves a job by ID.
func (r *JobRepository) GetByID(ctx context.Context, id string) (*domain.Job, error) {
	j, err := scanJob(r.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("job not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	return j, nil
}

// GetByCanonicalURL retrieves a job by its canonical URL, returning (nil,
// nil) if no such job exists.
func (r *JobRepository) GetByCanonicalURL(ctx context.Context, url string) (*domain.Job, error) {
	j, err := scanJob(r.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE canonical_url = ?`, url))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get job by URL: %w", err)
	}
	return j, nil
}

// jobFilterClause builds the WHERE clause and arguments that restrict a job
// query to rows matching filter. Even an empty filter leaves out the jobs
// the user ruled out; see domain.JobInterest.
//
// Every keyword term must appear, in the title or the company name, in any order —
// the same rule domain.SearchCriteria.Matches applies during discovery, and
// for the same reason: browsing stored jobs and searching sources must not
// disagree about what a query means. Matching terms rather than the whole
// string is what lets "backend engineer" find "Engineer, Backend (Go)".
//
// Terms are bound as parameters, never interpolated: they come straight
// from a search box.
//
// The posting date is compared through julianday, which reads the stored
// form (see connectionPragmas) whatever its precision. A job stored without
// a posting date holds the zero time, and falls back to its discovery date.
func jobFilterClause(filter domain.JobFilter) (string, []any) {
	var clauses []string
	var args []any

	for _, term := range domain.SearchTerms(filter.Keywords) {
		clauses = append(clauses, "(LOWER(title) LIKE ? OR LOWER(company_name) LIKE ?)")
		pattern := "%" + term + "%"
		args = append(args, pattern, pattern)
	}
	if filter.SourceID != "" {
		clauses = append(clauses, "source_id = ?")
		args = append(args, filter.SourceID)
	}
	if filter.RemoteType != "" {
		clauses = append(clauses, "LOWER(remote_type) = ?")
		args = append(args, filter.RemoteType)
	}
	switch filter.Interest {
	case domain.JobsInterested:
		clauses = append(clauses, "not_interested_at IS NULL")
	case domain.JobsNotInterested:
		clauses = append(clauses, "not_interested_at IS NOT NULL")
	}
	if !filter.PostedSince.IsZero() {
		clauses = append(clauses, `julianday(CASE
			WHEN julianday(posted_at) > julianday('1970-01-01') THEN posted_at
			ELSE discovered_at END) >= julianday(?)`)
		args = append(args, filter.PostedSince)
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// Count returns how many stored jobs match filter. Callers paginating
// through List use it to tell whether more pages remain.
func (r *JobRepository) Count(ctx context.Context, filter domain.JobFilter) (int, error) {
	where, args := jobFilterClause(filter)

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs"+where, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count jobs: %w", err)
	}
	return total, nil
}

// List returns stored jobs ordered by discovery time, most recent first,
// optionally filtered and paginated by limit/offset. It reads only local
// history and never contacts a job source, so it works offline.
func (r *JobRepository) List(ctx context.Context, filter domain.JobFilter, limit, offset int) ([]*domain.Job, error) {
	where, args := jobFilterClause(filter)

	// The jobs the user ruled out are listed by when they were ruled out,
	// the order the "Not interested" view reads them in.
	order := ` ORDER BY discovered_at DESC`
	if filter.Interest == domain.JobsNotInterested {
		order = ` ORDER BY not_interested_at DESC`
	}
	query := `SELECT ` + jobColumns + ` FROM jobs` + where + order + ` LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, append(args, limit, offset)...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]*domain.Job, 0)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job: %w", err)
		}
		jobs = append(jobs, j)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}

	return jobs, nil
}

// SetNotInterested marks a job not interested as of at, or clears the mark
// when at is nil.
func (r *JobRepository) SetNotInterested(ctx context.Context, id string, at *time.Time) error {
	var value any
	if at != nil {
		value = *at
	}
	result, err := r.db.ExecContext(ctx, `UPDATE jobs SET not_interested_at = ? WHERE id = ?`, value, id)
	if err != nil {
		return fmt.Errorf("failed to mark the job: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("failed to mark the job: %w", err)
	} else if n == 0 {
		return fmt.Errorf("job not found: %s", id)
	}
	return nil
}
