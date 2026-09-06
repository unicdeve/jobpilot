package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SettingsRepository is a ports.SettingsRepository backed by SQLite, storing
// arbitrary key/value settings in the user_settings table.
type SettingsRepository struct {
	db *sql.DB
}

// NewSettingsRepository constructs a SettingsRepository over db.
func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

// Get retrieves the value stored under key. found is false if no row exists
// for key; that is not treated as an error.
func (r *SettingsRepository) Get(ctx context.Context, key string) (string, bool, error) {
	query := `SELECT value FROM user_settings WHERE key = ?`

	var value string
	err := r.db.QueryRowContext(ctx, query, key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get setting %q: %w", key, err)
	}

	return value, true, nil
}

// Set stores value under key, creating or overwriting the row as needed.
func (r *SettingsRepository) Set(ctx context.Context, key, value string) error {
	query := `INSERT INTO user_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`

	if _, err := r.db.ExecContext(ctx, query, key, value); err != nil {
		return fmt.Errorf("failed to set setting %q: %w", key, err)
	}

	return nil
}
