package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	// Registers the "sqlite" driver with database/sql.
	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection and its schema lifecycle.
type DB struct {
	conn *sql.DB
}

// NewDB opens a SQLite database at dbPath and enables foreign key
// enforcement.
func NewDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", dbPath, err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &DB{conn: db}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn exposes the underlying *sql.DB for use by repository adapters.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// migrationProvider builds the goose provider over the embedded migrations
// in migrations/, shared by InitializeSchema and the migrate dev tool
// (cmd/migrate) so both use the exact same migration set and dialect.
func (db *DB) migrationProvider() (*goose.Provider, error) {
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded migrations: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db.conn, migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to create migration provider: %w", err)
	}

	return provider, nil
}

// InitializeSchema brings the database up to date with the embedded goose
// migrations in migrations/. It is safe to call on every startup: goose
// tracks applied migrations in a goose_db_version table and only runs new
// ones.
func (db *DB) InitializeSchema(ctx context.Context) error {
	provider, err := db.migrationProvider()
	if err != nil {
		return err
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// MigrateDown rolls back the most recently applied migration. It is a
// development helper (see cmd/migrate), not something the application
// itself calls.
func (db *DB) MigrateDown(ctx context.Context) (*goose.MigrationResult, error) {
	provider, err := db.migrationProvider()
	if err != nil {
		return nil, err
	}

	result, err := provider.Down(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to roll back migration: %w", err)
	}

	return result, nil
}

// MigrationStatus reports which embedded migrations have been applied. It
// is a development helper (see cmd/migrate), not something the application
// itself calls.
func (db *DB) MigrationStatus(ctx context.Context) ([]*goose.MigrationStatus, error) {
	provider, err := db.migrationProvider()
	if err != nil {
		return nil, err
	}

	statuses, err := provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get migration status: %w", err)
	}

	return statuses, nil
}
