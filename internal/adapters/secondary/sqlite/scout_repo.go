package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
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
	source, is_enabled, schedule, every_hours, daily_at, schedule_set_at,
	last_run_at, created_at, updated_at`

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
	schedule := scout.Schedule.Normalized()

	_, err := r.db.ExecContext(ctx, `INSERT INTO scouts (`+scoutColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		scout.ID, scout.Name, scout.SavedSearchID, scout.MinMatchScore,
		scout.MaxApplicationsPerRun, string(scout.Source), scout.IsEnabled,
		string(schedule.Kind), schedule.EveryHours, schedule.DailyAt, scout.ScheduleSetAt,
		scout.LastRunAt, scout.CreatedAt, scout.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save scout: %w", err)
	}
	return nil
}

// Update persists what the user can change about a scout.
//
// It does not write last_run_at: an edit is not a run, and a run that
// finishes while the user is editing must not be undone by their save, nor
// their save by it. MarkRun records a run.
func (r *ScoutRepository) Update(ctx context.Context, scout *domain.Scout) error {
	scout.UpdatedAt = time.Now()
	schedule := scout.Schedule.Normalized()

	result, err := r.db.ExecContext(ctx, `UPDATE scouts SET
		name = ?, saved_search_id = ?, min_match_score = ?,
		max_applications_per_run = ?, source = ?, is_enabled = ?,
		schedule = ?, every_hours = ?, daily_at = ?, schedule_set_at = ?,
		updated_at = ?
		WHERE id = ?`,
		scout.Name, scout.SavedSearchID, scout.MinMatchScore,
		scout.MaxApplicationsPerRun, string(scout.Source), scout.IsEnabled,
		string(schedule.Kind), schedule.EveryHours, schedule.DailyAt, scout.ScheduleSetAt,
		scout.UpdatedAt, scout.ID)
	if err != nil {
		return fmt.Errorf("failed to update scout: %w", err)
	}
	return requireAffected(result, scout.ID)
}

// MarkRun records when a scout's run finished, and touches nothing else.
func (r *ScoutRepository) MarkRun(ctx context.Context, id string, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE scouts SET last_run_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("failed to record the scout's run: %w", err)
	}
	return requireAffected(result, id)
}

func requireAffected(result sql.Result, id string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update scout: %w", err)
	}
	if affected == 0 {
		return &ports.NotFoundError{Kind: "scout", ID: id}
	}
	return nil
}

func scanScout(scanner interface{ Scan(...any) error }) (*domain.Scout, error) {
	var scout domain.Scout
	var source, schedule string
	if err := scanner.Scan(&scout.ID, &scout.Name, &scout.SavedSearchID,
		&scout.MinMatchScore, &scout.MaxApplicationsPerRun, &source,
		&scout.IsEnabled, &schedule, &scout.Schedule.EveryHours, &scout.Schedule.DailyAt,
		&scout.ScheduleSetAt, &scout.LastRunAt, &scout.CreatedAt, &scout.UpdatedAt); err != nil {
		return nil, err
	}
	scout.Source = domain.ScoutSource(source)
	scout.Schedule.Kind = domain.ScheduleKind(schedule)
	scout.Schedule = scout.Schedule.Normalized()
	return &scout, nil
}

// GetByID retrieves a scout.
func (r *ScoutRepository) GetByID(ctx context.Context, id string) (*domain.Scout, error) {
	row := r.db.QueryRowContext(ctx, `SELECT`+scoutColumns+` FROM scouts WHERE id = ?`, id)

	scout, err := scanScout(row)
	if err == sql.ErrNoRows {
		return nil, &ports.NotFoundError{Kind: "scout", ID: id}
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
