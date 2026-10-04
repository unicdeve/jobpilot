package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
)

// goMigrations are the migrations SQL cannot express, registered beside the
// embedded SQL files and numbered in the same sequence.
func goMigrations() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(14, &goose.GoFunc{RunTx: normalizeTimestamps}, nil),
		goose.NewGoMigration(15, &goose.GoFunc{RunDB: installDeletionRules}, nil),
		goose.NewGoMigration(24, &goose.GoFunc{RunTx: versionOpenAICompatBaseURL}, nil),
	}
}

// storedTimeLayout is how every time is written from migration 00014 on
// (see connectionPragmas): UTC, in a form SQLite's date functions read.
const storedTimeLayout = "2006-01-02 15:04:05.999999999-07:00"

// legacyTimeLayouts are the forms a stored time can be in before 00014:
// the driver's old default, time.Time.String with its monotonic reading cut
// off, and whatever SQLite's CURRENT_TIMESTAMP or the driver's own parser
// accepts. A layout without a zone is UTC, as CURRENT_TIMESTAMP is.
var legacyTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// parseLegacyTime reads a time stored in any of legacyTimeLayouts.
func parseLegacyTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if i := strings.Index(value, " m="); i >= 0 {
		value = value[:i]
	}
	for _, layout := range legacyTimeLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q", value)
}

// normalizeTimestamps rewrites every DATETIME value in the database into
// storedTimeLayout, in UTC. Before it, times were stored in the zone they
// were created in and in a form no SQL date function reads, so a query
// could neither compare them nor filter on them.
//
// It finds the columns from the schema rather than a list, so a DATETIME
// column no one remembered is not left behind. A value it cannot read
// stops the migration: guessing would silently move a date.
func normalizeTimestamps(ctx context.Context, tx *sql.Tx) error {
	tables, err := queryStrings(ctx, tx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'goose_db_version'`)
	if err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}

	for _, table := range tables {
		columns, err := datetimeColumns(ctx, tx, table)
		if err != nil {
			return err
		}
		for _, column := range columns {
			if err := normalizeColumn(ctx, tx, table, column); err != nil {
				return err
			}
		}
	}
	return nil
}

func datetimeColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name, type FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, fmt.Errorf("failed to read columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			return nil, fmt.Errorf("failed to read columns of %s: %w", table, err)
		}
		if strings.EqualFold(kind, "DATETIME") {
			columns = append(columns, name)
		}
	}
	return columns, rows.Err()
}

func normalizeColumn(ctx context.Context, tx *sql.Tx, table, column string) error {
	// Table and column names come from the schema itself, never from input,
	// and are quoted as identifiers.
	ident := func(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(
		`SELECT rowid, CAST(%[1]s AS TEXT) FROM %[2]s WHERE typeof(%[1]s) = 'text'`, ident(column), ident(table)))
	if err != nil {
		return fmt.Errorf("failed to read %s.%s: %w", table, column, err)
	}

	updates := map[int64]string{}
	for rows.Next() {
		var rowid int64
		var value string
		if err := rows.Scan(&rowid, &value); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to read %s.%s: %w", table, column, err)
		}
		t, err := parseLegacyTime(value)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("%s.%s, row %d: %w", table, column, rowid, err)
		}
		if normalized := t.UTC().Format(storedTimeLayout); normalized != value {
			updates[rowid] = normalized
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read %s.%s: %w", table, column, err)
	}

	update := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE rowid = ?`, ident(table), ident(column))
	for rowid, value := range updates {
		if _, err := tx.ExecContext(ctx, update, value, rowid); err != nil {
			return fmt.Errorf("failed to update %s.%s: %w", table, column, err)
		}
	}
	return nil
}

func queryStrings(ctx context.Context, tx *sql.Tx, query string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
