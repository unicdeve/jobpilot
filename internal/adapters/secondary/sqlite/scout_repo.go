package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// ScoutRepository persists Scouts in SQLite.
type ScoutRepository struct {
	db *sql.DB
}

// NewScoutRepository constructs a ScoutRepository over db.
func NewScoutRepository(db *sql.DB) *ScoutRepository {
	return &ScoutRepository{db: db}
}

const scoutColumns = `
	id, name, saved_search_id, min_match_score, max_applications_per_run,
	source, is_enabled, last_run_at, created_at, updated_at`

// Save inserts a new scout, assigning an ID and timestamps.
func (r *ScoutRepository) Save(ctx context.Context, scout *domain.Scout) error {
	if scout.ID == "" {
		scout.ID = uuid.NewString()
	}
	now := time.Now()
	if scout.CreatedAt.IsZero() {
		scout.CreatedAt = now
	}
	scout.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `INSERT INTO scouts (`+scoutColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		scout.ID, scout.Name, scout.SavedSearchID, scout.MinMatchScore,
		scout.MaxApplicationsPerRun, string(scout.Source), scout.IsEnabled,
		scout.LastRunAt, scout.CreatedAt, scout.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save scout: %w", err)
	}
	return nil
}

// Update persists a scout's mutable fields.
func (r *ScoutRepository) Update(ctx context.Context, scout *domain.Scout) error {
	scout.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, `UPDATE scouts SET
		name = ?, saved_search_id = ?, min_match_score = ?,
		max_applications_per_run = ?, source = ?, is_enabled = ?,
		last_run_at = ?, updated_at = ?
		WHERE id = ?`,
		scout.Name, scout.SavedSearchID, scout.MinMatchScore,
		scout.MaxApplicationsPerRun, string(scout.Source), scout.IsEnabled,
		scout.LastRunAt, scout.UpdatedAt, scout.ID)
	if err != nil {
		return fmt.Errorf("failed to update scout: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update scout: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("scout not found: %s", scout.ID)
	}
	return nil
}

func scanScout(scanner interface{ Scan(...any) error }) (*domain.Scout, error) {
	var scout domain.Scout
	var source string
	if err := scanner.Scan(&scout.ID, &scout.Name, &scout.SavedSearchID,
		&scout.MinMatchScore, &scout.MaxApplicationsPerRun, &source,
		&scout.IsEnabled, &scout.LastRunAt, &scout.CreatedAt, &scout.UpdatedAt); err != nil {
		return nil, err
	}
	scout.Source = domain.ScoutSource(source)
	return &scout, nil
}

// GetByID retrieves a scout.
func (r *ScoutRepository) GetByID(ctx context.Context, id string) (*domain.Scout, error) {
	row := r.db.QueryRowContext(ctx, `SELECT`+scoutColumns+` FROM scouts WHERE id = ?`, id)

	scout, err := scanScout(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scout not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scout: %w", err)
	}
	return scout, nil
}

// List returns every scout, newest first.
func (r *ScoutRepository) List(ctx context.Context) ([]*domain.Scout, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT`+scoutColumns+` FROM scouts ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list scouts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var scouts []*domain.Scout
	for rows.Next() {
		scout, err := scanScout(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan scout: %w", err)
		}
		scouts = append(scouts, scout)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read scouts: %w", err)
	}
	return scouts, nil
}

// Delete removes a scout.
func (r *ScoutRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM scouts WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete scout: %w", err)
	}
	return nil
}
