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

// WatchedCompanyRepository is a ports.WatchedCompanyRepository backed by
// SQLite.
type WatchedCompanyRepository struct {
	db *sql.DB
}

// NewWatchedCompanyRepository constructs a WatchedCompanyRepository over db.
func NewWatchedCompanyRepository(db *sql.DB) *WatchedCompanyRepository {
	return &WatchedCompanyRepository{db: db}
}

const watchedCompanyColumns = `id, provider, region, board, name, added_from, added_at,
	checked_at, check_status, open_roles`

// Create stores a new company, or returns the one already watching its
// board. The unique index decides, so two requests to watch one board at
// once store it once.
func (r *WatchedCompanyRepository) Create(ctx context.Context, company *domain.WatchedCompany) (*domain.WatchedCompany, error) {
	if company.ID == "" {
		company.ID = uuid.New().String()
	}
	if company.AddedAt.IsZero() {
		company.AddedAt = time.Now().UTC()
	}

	const query = `INSERT INTO watched_companies (id, provider, region, board, name, added_from, added_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(provider, region, board COLLATE NOCASE) DO NOTHING`

	result, err := r.db.ExecContext(ctx, query, company.ID, company.Board.Provider, company.Board.Region,
		company.Board.Name, company.Name, company.AddedFrom, company.AddedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to watch %s: %w", company.Name, err)
	}
	if inserted, err := result.RowsAffected(); err == nil && inserted == 0 {
		existing, err := r.GetByBoard(ctx, company.Board)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}
	return r.GetByID(ctx, company.ID)
}

func scanWatchedCompany(row interface{ Scan(...any) error }) (*domain.WatchedCompany, error) {
	var company domain.WatchedCompany
	var checkedAt sql.NullTime
	var provider, check string
	err := row.Scan(&company.ID, &provider, &company.Board.Region, &company.Board.Name, &company.Name,
		&company.AddedFrom, &company.AddedAt, &checkedAt, &check, &company.OpenRoles)
	if err != nil {
		return nil, err
	}
	company.Board.Provider = domain.BoardProvider(provider)
	company.Check = domain.BoardCheck(check)
	company.URL = company.Board.URL()
	if checkedAt.Valid {
		company.CheckedAt = &checkedAt.Time
	}
	return &company, nil
}

// GetByID returns one company, or ports.ErrWatchedCompanyNotFound.
func (r *WatchedCompanyRepository) GetByID(ctx context.Context, id string) (*domain.WatchedCompany, error) {
	query := `SELECT ` + watchedCompanyColumns + ` FROM watched_companies WHERE id = ?`
	company, err := scanWatchedCompany(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ports.ErrWatchedCompanyNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get watched company %s: %w", id, err)
	}
	return company, nil
}

// GetByBoard returns the company watching board, or nil.
func (r *WatchedCompanyRepository) GetByBoard(ctx context.Context, board domain.Board) (*domain.WatchedCompany, error) {
	query := `SELECT ` + watchedCompanyColumns + ` FROM watched_companies
	WHERE provider = ? AND region = ? AND board = ? COLLATE NOCASE`
	company, err := scanWatchedCompany(r.db.QueryRowContext(ctx, query, board.Provider, board.Region, board.Name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find the company watching %s: %w", board.URL(), err)
	}
	return company, nil
}

// List returns every company, by name.
func (r *WatchedCompanyRepository) List(ctx context.Context) ([]*domain.WatchedCompany, error) {
	query := `SELECT ` + watchedCompanyColumns + ` FROM watched_companies ORDER BY name COLLATE NOCASE, id`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list watched companies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	companies := make([]*domain.WatchedCompany, 0)
	for rows.Next() {
		company, err := scanWatchedCompany(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to read a watched company: %w", err)
		}
		companies = append(companies, company)
	}
	return companies, rows.Err()
}

// Rename changes a company's name.
func (r *WatchedCompanyRepository) Rename(ctx context.Context, id, name string) error {
	return r.exec(ctx, id, `UPDATE watched_companies SET name = ? WHERE id = ?`, name, id)
}

// RecordCheck stores how reading a company's board went.
func (r *WatchedCompanyRepository) RecordCheck(ctx context.Context, id string, at time.Time, check domain.BoardCheck, openRoles int) error {
	return r.exec(ctx, id, `UPDATE watched_companies SET checked_at = ?, check_status = ?, open_roles = ? WHERE id = ?`,
		at.UTC(), check, openRoles, id)
}

// Delete removes a company.
func (r *WatchedCompanyRepository) Delete(ctx context.Context, id string) error {
	return r.exec(ctx, id, `DELETE FROM watched_companies WHERE id = ?`, id)
}

// exec runs a statement about one company, and returns
// ports.ErrWatchedCompanyNotFound when it touched none.
func (r *WatchedCompanyRepository) exec(ctx context.Context, id, query string, args ...any) error {
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update watched company %s: %w", id, err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %s", ports.ErrWatchedCompanyNotFound, id)
	}
	return nil
}
