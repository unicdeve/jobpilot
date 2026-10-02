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

// SavedSearchRepository is a ports.SavedSearchRepository backed by SQLite.
type SavedSearchRepository struct {
	db *sql.DB
}

// NewSavedSearchRepository constructs a SavedSearchRepository over db.
func NewSavedSearchRepository(db *sql.DB) *SavedSearchRepository {
	return &SavedSearchRepository{db: db}
}

const savedSearchColumns = `id, name, keywords, title, location, remote_preference,
	employment_type, salary_min, salary_max, excluded_companies, work_from,
	include_relocation, fit_hours, hours_shift, is_default, created_at, updated_at`

// savedSearchSelect is what is read back: the written columns plus the
// last run, which only MarkRun writes.
const savedSearchSelect = savedSearchColumns + `, last_run_at`

// Save inserts a new saved search or updates an existing one, assigning an
// ID when the search does not already have one.
// An edit preserves the stored default status so a stale read cannot undo
// a concurrent SetDefault.
func (r *SavedSearchRepository) Save(ctx context.Context, search *domain.SavedSearch) error {
	if search.ID == "" {
		search.ID = uuid.New().String()
	}

	query := `INSERT INTO saved_searches (` + savedSearchColumns + `)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		keywords = excluded.keywords,
		title = excluded.title,
		location = excluded.location,
		remote_preference = excluded.remote_preference,
		employment_type = excluded.employment_type,
		salary_min = excluded.salary_min,
		salary_max = excluded.salary_max,
		excluded_companies = excluded.excluded_companies,
		work_from = excluded.work_from,
		include_relocation = excluded.include_relocation,
		fit_hours = excluded.fit_hours,
		hours_shift = excluded.hours_shift,
		updated_at = CURRENT_TIMESTAMP`

	c := search.Criteria
	_, err := r.db.ExecContext(ctx, query,
		search.ID, search.Name, c.Keywords, c.Title, c.Location, c.RemotePreference,
		c.EmploymentType, c.SalaryMin, c.SalaryMax, joinStrings(c.ExcludedCompanies),
		c.WorkFrom, c.IncludeRelocation, c.FitHours, c.HoursShift, search.IsDefault,
	)
	if err != nil {
		return fmt.Errorf("failed to save search %q: %w", search.Name, err)
	}
	return nil
}

func (r *SavedSearchRepository) scan(row interface{ Scan(...any) error }) (*domain.SavedSearch, error) {
	var s domain.SavedSearch
	var excluded string
	var lastRun sql.NullTime

	err := row.Scan(
		&s.ID, &s.Name, &s.Criteria.Keywords, &s.Criteria.Title, &s.Criteria.Location,
		&s.Criteria.RemotePreference, &s.Criteria.EmploymentType, &s.Criteria.SalaryMin,
		&s.Criteria.SalaryMax, &excluded, &s.Criteria.WorkFrom, &s.Criteria.IncludeRelocation,
		&s.Criteria.FitHours, &s.Criteria.HoursShift,
		&s.IsDefault, &s.CreatedAt, &s.UpdatedAt, &lastRun,
	)
	if err != nil {
		return nil, err
	}

	s.Criteria.ExcludedCompanies = splitStrings(excluded)
	if lastRun.Valid {
		s.LastRunAt = &lastRun.Time
	}
	return &s, nil
}

// GetByID returns one saved search. A missing row is an error: a caller
// asking for a specific search by ID expects it to exist.
func (r *SavedSearchRepository) GetByID(ctx context.Context, id string) (*domain.SavedSearch, error) {
	query := `SELECT ` + savedSearchSelect + ` FROM saved_searches WHERE id = ?`

	search, err := r.scan(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ports.NotFoundError{Kind: "saved search", ID: id}
		}
		return nil, fmt.Errorf("failed to get saved search %s: %w", id, err)
	}
	return search, nil
}

// GetByName returns the saved search with this name, or (nil, nil) if there
// is none — callers use it to check a name is free, where absence is the
// expected result rather than a failure.
func (r *SavedSearchRepository) GetByName(ctx context.Context, name string) (*domain.SavedSearch, error) {
	query := `SELECT ` + savedSearchSelect + ` FROM saved_searches WHERE name = ? COLLATE NOCASE`

	search, err := r.scan(r.db.QueryRowContext(ctx, query, name))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get saved search %q: %w", name, err)
	}
	return search, nil
}

// List returns every saved search, the default first and the rest by name,
// so the one discovery would run without being told otherwise is at the top.
func (r *SavedSearchRepository) List(ctx context.Context) ([]*domain.SavedSearch, error) {
	query := `SELECT ` + savedSearchSelect + ` FROM saved_searches
		ORDER BY is_default DESC, name COLLATE NOCASE ASC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list saved searches: %w", err)
	}
	defer func() { _ = rows.Close() }()

	searches := make([]*domain.SavedSearch, 0)
	for rows.Next() {
		search, err := r.scan(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan saved search: %w", err)
		}
		searches = append(searches, search)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating saved searches: %w", err)
	}
	return searches, nil
}

// Delete removes a saved search and promotes a replacement default in one
// transaction. The first statement takes the write lock before choosing
// a replacement, so concurrent default changes cannot interleave.
func (r *SavedSearchRepository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin saved search deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var wasDefault bool
	err = tx.QueryRowContext(ctx, `DELETE FROM saved_searches WHERE id = ? RETURNING is_default`, id).Scan(&wasDefault)
	if errors.Is(err, sql.ErrNoRows) {
		return &ports.NotFoundError{Kind: "saved search", ID: id}
	}
	if err != nil {
		return fmt.Errorf("failed to delete saved search %s: %w", id, err)
	}
	if wasDefault {
		_, err := tx.ExecContext(ctx, `UPDATE saved_searches
			SET is_default = 1, updated_at = CURRENT_TIMESTAMP
			WHERE id = (SELECT id FROM saved_searches ORDER BY name COLLATE NOCASE, id LIMIT 1)`)
		if err != nil {
			return fmt.Errorf("failed to promote replacement saved search: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit saved search deletion: %w", err)
	}
	return nil
}

// SetDefault promotes a search atomically, preserving the old default if
// the target is missing or any write fails.
func (r *SavedSearchRepository) SetDefault(ctx context.Context, id string) (*domain.SavedSearch, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin default saved search change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Write before reading to serialize competing changes under SQLite's
	// write lock, without upgrading a potentially stale read snapshot.
	_, err = tx.ExecContext(ctx,
		`UPDATE saved_searches SET is_default = 0, updated_at = CURRENT_TIMESTAMP WHERE is_default = 1`)
	if err != nil {
		return nil, fmt.Errorf("failed to clear the default saved search: %w", err)
	}
	search, err := r.scan(tx.QueryRowContext(ctx, `UPDATE saved_searches
		SET is_default = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING `+savedSearchSelect, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &ports.NotFoundError{Kind: "saved search", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to promote saved search %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit default saved search change: %w", err)
	}
	return search, nil
}

// MarkRun records when a search last ran. A search deleted meanwhile is not
// an error: there is nothing left to be stale.
func (r *SavedSearchRepository) MarkRun(ctx context.Context, id string, at time.Time) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE saved_searches SET last_run_at = ? WHERE id = ?`, at, id); err != nil {
		return fmt.Errorf("failed to record the run of saved search %s: %w", id, err)
	}
	return nil
}
