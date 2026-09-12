package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	"github.com/pressly/goose/v3"

	// Registers the "sqlite" driver with database/sql.
	_ "modernc.org/sqlite"
)

// DB wraps a SQLite connection and its schema lifecycle.
type DB struct {
	conn *sql.DB
}

// connectionPragmas are applied to every connection database/sql opens, not
// just the first one. Setting them through the DSN rather than a one-off
// db.Exec matters because database/sql keeps a pool: a PRAGMA executed via
// db.Exec lands on whichever pooled connection it happened to get, leaving
// the rest of the pool unconfigured.
//
//   - busy_timeout: without it, a connection that finds the database locked
//     fails immediately with SQLITE_BUSY instead of waiting for the lock to
//     clear. Concurrent API requests (the desktop app opens several at once
//     on startup) made that a user-visible "database is locked" error.
//   - journal_mode=WAL: in the default rollback-journal mode a writer blocks
//     all readers. WAL lets readers proceed while a write is in flight, which
//     is the normal shape of this application's load — one writer saving
//     discovered jobs while screens read.
//   - foreign_keys: SQLite defaults this off and it is per-connection, so
//     without it here the schema's foreign keys are only enforced on whatever
//     connection happened to run the pragma.
var connectionPragmas = url.Values{
	"_busy_timeout": {"5000"},
	"_journal_mode": {"WAL"},
	"_foreign_keys": {"on"},
}

// dsn builds the modernc.org/sqlite connection string for dbPath.
//
// The path is deliberately not given a "file:" scheme prefix. The driver
// splits the DSN at the first "?" and, for a non-"file:" DSN, hands the
// path to SQLite verbatim — so no percent-encoding is needed and Windows
// paths with backslashes or a drive letter work unchanged. The one case
// that cannot work is a path containing a literal "?", which the driver
// would read as the start of the query string; that is rejected rather
// than silently opening a different database file.
func dsn(dbPath string) (string, error) {
	if strings.Contains(dbPath, "?") {
		return "", fmt.Errorf("database path %q contains '?', which sqlite reads as the start of connection parameters", dbPath)
	}
	return dbPath + "?" + connectionPragmas.Encode(), nil
}

// NewDB opens a SQLite database at dbPath with the connection pragmas every
// connection in the pool needs (see connectionPragmas).
func NewDB(dbPath string) (*DB, error) {
	connString, err := dsn(dbPath)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", connString)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", dbPath, err)
	}

	// sql.Open is lazy: it does not connect, so a bad path or an invalid
	// pragma would otherwise surface much later, at the first query.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to sqlite database at %s: %w", dbPath, err)
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
