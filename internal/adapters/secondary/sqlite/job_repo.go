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
	"github.com/unicdev/jobpilot/internal/core/ports"
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
		posted_at, expires_at, discovered_at, updated_at, raw_source_reference,
		` + signalColumns + `
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	args := []any{
		job.ID, job.CanonicalURL, job.SourceID, job.SourceJobID, job.Title, job.CompanyName,
		job.CompanyURL, job.DescriptionMarkdown, joinStrings(job.Locations), job.RemoteType,
		job.EmploymentType, job.SalaryMin, job.SalaryMax, job.SalaryCurrency,
		job.SalaryPeriod, job.ExperienceMin, job.ExperienceMax, joinStrings(job.Skills),
		job.PostedAt, job.ExpiresAt, job.DiscoveredAt, job.UpdatedAt, job.RawSourceReference,
	}
	_, err := r.db.ExecContext(ctx, query, append(args, signalValues(job)...)...)
	if err != nil {
		return fmt.Errorf("failed to save job: %w", err)
	}

	return nil
}

// signalColumns are what domain.ReadSignals derives (ADR-0023 §3,
// ADR-0025), in the order signalValues writes them and scanJob reads them.
const signalColumns = `workable_scope, workable_places, workable_countries, workable_note,
	relocation, relocation_evidence, visa_sponsorship, visa_evidence,
	hours_status, hours_from, hours_to, hours_evidence, signals_version`

// signalValues are a job's signals as signalColumns stores them.
func signalValues(job *domain.Job) []any {
	return []any{
		string(job.Workable.Scope), joinStrings(job.Workable.Places),
		joinCountries(job.Workable.Countries), job.Workable.Note,
		string(job.Relocation.Status), job.Relocation.Evidence,
		string(job.VisaSponsorship.Status), job.VisaSponsorship.Evidence,
		string(job.Hours.Status), job.Hours.From, job.Hours.To, job.Hours.Evidence,
		job.SignalsVersion,
	}
}

// joinCountries stores ISO codes as ",DE,FR,", so that one country is the
// pattern ",DE," whichever position it holds.
func joinCountries(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	return "," + strings.Join(codes, ",") + ","
}

func splitCountries(stored string) []string {
	trimmed := strings.Trim(stored, ",")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, ",")
}

// jobColumns are the columns scanJob reads, in its order.
const jobColumns = `id, canonical_url, source_id, source_job_id, title, company_name,
	company_url, description_markdown, locations, remote_type,
	employment_type, salary_min, salary_max, salary_currency,
	salary_period, experience_min, experience_max, skills,
	posted_at, expires_at, discovered_at, updated_at, raw_source_reference,
	not_interested_at, ` + signalColumns

// rowScanner is what *sql.Row and *sql.Rows have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanJob reads a job selected with jobColumns.
func scanJob(row rowScanner) (*domain.Job, error) {
	var j domain.Job
	var locations, skills, places, countries string
	var notInterested sql.NullTime
	err := row.Scan(
		&j.ID, &j.CanonicalURL, &j.SourceID, &j.SourceJobID, &j.Title, &j.CompanyName,
		&j.CompanyURL, &j.DescriptionMarkdown, &locations, &j.RemoteType,
		&j.EmploymentType, &j.SalaryMin, &j.SalaryMax, &j.SalaryCurrency,
		&j.SalaryPeriod, &j.ExperienceMin, &j.ExperienceMax, &skills,
		&j.PostedAt, &j.ExpiresAt, &j.DiscoveredAt, &j.UpdatedAt, &j.RawSourceReference,
		&notInterested,
		&j.Workable.Scope, &places, &countries, &j.Workable.Note,
		&j.Relocation.Status, &j.Relocation.Evidence,
		&j.VisaSponsorship.Status, &j.VisaSponsorship.Evidence,
		&j.Hours.Status, &j.Hours.From, &j.Hours.To, &j.Hours.Evidence, &j.SignalsVersion,
	)
	if err != nil {
		return nil, err
	}
	j.Locations = splitStrings(locations)
	j.Skills = splitStrings(skills)
	j.Workable.Places = splitStrings(places)
	j.Workable.Countries = splitCountries(countries)
	if j.Workable.Scope == "" {
		// Not read yet: the daemon reads it when it starts (ADR-0023 §3).
		j.Workable.Scope = domain.WorkableUnknown
	}
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
			return nil, &ports.NotFoundError{Kind: "job", ID: id}
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
	if clause, workArgs := workableClause(filter); clause != "" {
		clauses = append(clauses, clause)
		args = append(args, workArgs...)
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

// workableClause is domain.SearchCriteria.MatchesFor's rule for WorkFrom
// and IncludeRelocation, in SQL, so the stored list and a search agree on
// what "I can work from" keeps. domain.SearchCriteria's tests and this
// adapter's hold the two to the same cases.
func workableClause(filter domain.JobFilter) (string, []any) {
	from := strings.ToUpper(strings.TrimSpace(filter.WorkFrom))
	if from == "" {
		return "", nil
	}
	keep := []string{
		"workable_scope NOT IN ('anywhere', 'listed')",
		"workable_scope = 'anywhere'",
		"workable_countries LIKE ?",
	}
	args := []any{"%," + from + ",%"}
	if filter.IncludeRelocation {
		authorized := "0"
		if len(filter.AuthorizedCountries) > 0 {
			likes := make([]string, len(filter.AuthorizedCountries))
			for i, country := range filter.AuthorizedCountries {
				likes[i] = "workable_countries LIKE ?"
				args = append(args, "%,"+strings.ToUpper(country)+",%")
			}
			authorized = "(" + strings.Join(likes, " OR ") + ")"
		}
		keep = append(keep, `((relocation IN ('offered', 'mentioned') OR visa_sponsorship IN ('offered', 'mentioned'))
			AND (visa_sponsorship != 'declined' OR `+authorized+`))`)
	}
	return "(" + strings.Join(keep, " OR ") + ")", args
}

// ListUnread returns up to limit jobs read by a version of domain.ReadSignals
// older than version, the ones never read first.
func (r *JobRepository) ListUnread(ctx context.Context, version, limit int) ([]*domain.Job, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs
		WHERE signals_version < ? ORDER BY signals_version, id LIMIT ?`, version, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs to read: %w", err)
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

// SaveSignals stores what domain.ReadSignals read from a job, and nothing
// else about it.
func (r *JobRepository) SaveSignals(ctx context.Context, job *domain.Job) error {
	_, err := r.db.ExecContext(ctx, `UPDATE jobs SET
		workable_scope = ?, workable_places = ?, workable_countries = ?, workable_note = ?,
		relocation = ?, relocation_evidence = ?, visa_sponsorship = ?, visa_evidence = ?,
		hours_status = ?, hours_from = ?, hours_to = ?, hours_evidence = ?,
		signals_version = ?
		WHERE id = ?`, append(signalValues(job), job.ID)...)
	if err != nil {
		return fmt.Errorf("failed to store what job %s says: %w", job.ID, err)
	}
	return nil
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
		return &ports.NotFoundError{Kind: "job", ID: id}
	}
	return nil
}
