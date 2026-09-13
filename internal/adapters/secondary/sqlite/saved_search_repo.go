package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/unicdev/jobpilot/internal/core/domain"
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
	employment_type, salary_min, salary_max, excluded_companies, is_default,
	created_at, updated_at`

// Save inserts a new saved search or updates an existing one, assigning an
// ID when the search does not already have one.
func (r *SavedSearchRepository) Save(ctx context.Context, search *domain.SavedSearch) error {
	if search.ID == "" {
		search.ID = uuid.New().String()
	}

	query := `INSERT INTO saved_searches (` + savedSearchColumns + `)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
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
		is_default = excluded.is_default,
		updated_at = CURRENT_TIMESTAMP`

	c := search.Criteria
	_, err := r.db.ExecContext(ctx, query,
		search.ID, search.Name, c.Keywords, c.Title, c.Location, c.RemotePreference,
		c.EmploymentType, c.SalaryMin, c.SalaryMax, joinStrings(c.ExcludedCompanies),
		search.IsDefault,
	)
	if err != nil {
		return fmt.Errorf("failed to save search %q: %w", search.Name, err)
	}
	return nil
}

func (r *SavedSearchRepository) scan(row interface{ Scan(...any) error }) (*domain.SavedSearch, error) {
	var s domain.SavedSearch
	var excluded string

	err := row.Scan(
		&s.ID, &s.Name, &s.Criteria.Keywords, &s.Criteria.Title, &s.Criteria.Location,
		&s.Criteria.RemotePreference, &s.Criteria.EmploymentType, &s.Criteria.SalaryMin,
		&s.Criteria.SalaryMax, &excluded, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	s.Criteria.ExcludedCompanies = splitStrings(excluded)
	return &s, nil
}

// GetByID returns one saved search. A missing row is an error: a caller
// asking for a specific search by ID expects it to exist.
func (r *SavedSearchRepository) GetByID(ctx context.Context, id string) (*domain.SavedSearch, error) {
	query := `SELECT ` + savedSearchColumns + ` FROM saved_searches WHERE id = ?`

	search, err := r.scan(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("saved search not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get saved search %s: %w", id, err)
	}
	return search, nil
}

// GetByName returns the saved search with this name, or (nil, nil) if there
// is none — callers use it to check a name is free, where absence is the
// expected result rather than a failure.
func (r *SavedSearchRepository) GetByName(ctx context.Context, name string) (*domain.SavedSearch, error) {
	query := `SELECT ` + savedSearchColumns + ` FROM saved_searches WHERE name = ? COLLATE NOCASE`

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
	query := `SELECT ` + savedSearchColumns + ` FROM saved_searches
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

// Delete removes a saved search by ID.
func (r *SavedSearchRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM saved_searches WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete saved search %s: %w", id, err)
	}
	return nil
}

// ClearDefault unsets the default flag everywhere. Promoting a new default
// means clearing first: the schema permits only one row with is_default = 1.
func (r *SavedSearchRepository) ClearDefault(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE saved_searches SET is_default = 0, updated_at = CURRENT_TIMESTAMP WHERE is_default = 1`)
	if err != nil {
		return fmt.Errorf("failed to clear the default saved search: %w", err)
	}
	return nil
}
