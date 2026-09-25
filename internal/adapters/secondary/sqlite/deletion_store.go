package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// maxIDsPerStatement bounds an IN list. SQLite allows far more bound
// parameters than this, but a deletion's reach has no fixed size, and a
// statement per chunk keeps each one ordinary.
const maxIDsPerStatement = 500

// DeletionStore carries out deletions over this database (ADR-0021).
//
// It needs no list of which column references which: it reads the foreign
// keys from the schema and names their tables' records through
// recordTables. Deleting a record's row is enough, because those foreign
// keys carry domain.DeletionRules, which TestSchema_ForeignKeysAreTheDeletionRules
// proves.
type DeletionStore struct {
	db *sql.DB
}

// NewDeletionStore constructs a DeletionStore over db.
func NewDeletionStore(db *DB) *DeletionStore {
	return &DeletionStore{db: db.Conn()}
}

var _ ports.DeletionStore = (*DeletionStore)(nil)

// reference is one foreign key: a column of a table that references
// another table's records.
type reference struct {
	table  string
	column string
}

// InTransaction runs do in one transaction.
func (s *DeletionStore) InTransaction(ctx context.Context, do func(tx ports.DeletionTx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	references, err := readReferences(ctx, tx)
	if err != nil {
		return err
	}
	if err := do(&deletionTx{tx: tx, references: references}); err != nil {
		return err
	}
	return tx.Commit()
}

// Compact makes deleted content unreadable from the files on disk. With
// secure_delete on (see connectionPragmas) a deleted row is overwritten in
// the database file as it is deleted; the WAL still holds the pages as they
// were until it is checkpointed, and full additionally rewrites the file
// without its free pages (ADR-0021 §6).
func (s *DeletionStore) Compact(ctx context.Context, full bool) error {
	if full {
		if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
			return fmt.Errorf("failed to vacuum: %w", err)
		}
	}
	var busy, logFrames, checkpointed int
	if err := s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("failed to checkpoint the WAL: %w", err)
	}
	if busy != 0 {
		// A reader held the WAL open. The pages are overwritten the next
		// time a checkpoint completes; it is reported, not retried here.
		return fmt.Errorf("the WAL could not be checkpointed while another connection was reading it")
	}
	return nil
}

// readReferences indexes every foreign key by the kinds of record it
// relates, the terms domain.DeletionRules is written in.
func readReferences(ctx context.Context, tx *sql.Tx) (map[domain.DeletionRule][]reference, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT m.name, f."from", f."table", f.on_delete
		FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table'`)
	if err != nil {
		return nil, fmt.Errorf("failed to read the schema's foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	references := map[domain.DeletionRule][]reference{}
	for rows.Next() {
		var table, column, parent, onDelete string
		if err := rows.Scan(&table, &column, &parent, &onDelete); err != nil {
			return nil, fmt.Errorf("failed to read a foreign key: %w", err)
		}
		var effect domain.DeletionEffect
		switch onDelete {
		case "CASCADE":
			effect = domain.DeletionCascade
		case "SET NULL":
			effect = domain.DeletionDetach
		default:
			// A key with no rule implements none, so a rule that needs it
			// finds nothing and Referencing fails rather than understating.
			continue
		}
		rule := domain.DeletionRule{Referencing: recordTables[table], Referenced: recordTables[parent], Effect: effect}
		references[rule] = append(references[rule], reference{table: table, column: column})
	}
	return references, rows.Err()
}

// deletionTx is the database as one deletion sees it.
type deletionTx struct {
	tx         *sql.Tx
	references map[domain.DeletionRule][]reference
}

// tableOf is the one table that holds a kind of record the user deletes.
func tableOf(kind domain.RecordKind) (string, error) {
	var tables []string
	for table, k := range recordTables {
		if k == kind {
			tables = append(tables, table)
		}
	}
	if len(tables) != 1 {
		return "", fmt.Errorf("%s records are held in %d tables, not one", kind, len(tables))
	}
	return tables[0], nil
}

var targetKinds = map[domain.DeletionTarget]domain.RecordKind{
	domain.DeleteJob:         domain.RecordJob,
	domain.DeleteApplication: domain.RecordApplication,
	domain.DeleteCoverLetter: domain.RecordCoverLetter,
	domain.DeleteResume:      domain.RecordResume,
	domain.DeleteProfile:     domain.RecordCandidate,
	domain.ClearActivity:     domain.RecordAgentRun,
	domain.ClearJobs:         domain.RecordJob,
}

func (d *deletionTx) Roots(ctx context.Context, target domain.DeletionTarget, id string) (domain.RecordKind, []string, error) {
	kind, ok := targetKinds[target]
	if !ok {
		return "", nil, fmt.Errorf("%s has no records to start from", target)
	}
	table, err := tableOf(kind)
	if err != nil {
		return "", nil, err
	}

	var query string
	var args []any
	switch {
	case target.TakesID():
		query, args = `SELECT id FROM `+table+` WHERE id = ?`, []any{id}
	case target == domain.ClearJobs:
		// Every job no application uses, except those the user ruled out:
		// each of those holds a decision (spec D10).
		query = `SELECT id FROM jobs j WHERE j.not_interested_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.job_id = j.id)`
	default:
		query = `SELECT id FROM ` + table
	}
	ids, err := d.strings(ctx, query, args...)
	if err != nil {
		return "", nil, fmt.Errorf("failed to find what %s deletes: %w", target, err)
	}
	return kind, ids, nil
}

func (d *deletionTx) Referencing(ctx context.Context, rule domain.DeletionRule, ids []string) ([]string, error) {
	references, ok := d.references[rule]
	if !ok {
		// The agreement test makes this unreachable; if it is reached, a
		// preview would silently understate the reach, so it fails instead.
		return nil, fmt.Errorf("no foreign key implements the rule %s → %s (%s)", rule.Referencing, rule.Referenced, rule.Effect)
	}
	var found []string
	for _, ref := range references {
		err := chunked(ids, func(chunk []string) error {
			rows, err := d.strings(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE %s IN (%s)`,
				quoteIdent(ref.table), quoteIdent(ref.column), placeholders(len(chunk))), anys(chunk)...)
			found = append(found, rows...)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("failed to find the %s records referencing them: %w", ref.table, err)
		}
	}
	return found, nil
}

func (d *deletionTx) SentApplications(ctx context.Context, ids []string) (int, error) {
	total := 0
	err := chunked(ids, func(chunk []string) error {
		var n int
		err := d.tx.QueryRowContext(ctx, fmt.Sprintf(
			`SELECT count(*) FROM applications WHERE submitted_at IS NOT NULL AND id IN (%s)`, placeholders(len(chunk))),
			anys(chunk)...).Scan(&n)
		total += n
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count sent applications: %w", err)
	}
	return total, nil
}

func (d *deletionTx) KeptNotInterested(ctx context.Context) (int, error) {
	var n int
	err := d.tx.QueryRowContext(ctx, `SELECT count(*) FROM jobs j WHERE j.not_interested_at IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM applications a WHERE a.job_id = j.id)`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count jobs marked not interested: %w", err)
	}
	return n, nil
}

func (d *deletionTx) IsCurrentCoverLetter(ctx context.Context, id string) (bool, error) {
	var n int
	if err := d.tx.QueryRowContext(ctx, `SELECT count(*) FROM applications WHERE current_cover_letter_id = ?`, id).Scan(&n); err != nil {
		return false, fmt.Errorf("failed to check whether the cover letter is current: %w", err)
	}
	return n > 0, nil
}

func (d *deletionTx) Delete(ctx context.Context, kind domain.RecordKind, ids []string) error {
	table, err := tableOf(kind)
	if err != nil {
		return err
	}
	return chunked(ids, func(chunk []string) error {
		_, err := d.tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE id IN (%s)`, quoteIdent(table), placeholders(len(chunk))), anys(chunk)...)
		if err != nil {
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
		return nil
	})
}

func (d *deletionTx) CountAll(ctx context.Context) (map[domain.RecordKind]int, error) {
	counts := map[domain.RecordKind]int{}
	for table, kind := range recordTables {
		var n int
		if err := d.tx.QueryRowContext(ctx, `SELECT count(*) FROM `+quoteIdent(table)).Scan(&n); err != nil {
			return nil, fmt.Errorf("failed to count %s: %w", table, err)
		}
		if n > 0 {
			counts[kind] += n
		}
	}
	return counts, nil
}

// Fingerprint identifies the database's contents by each table's row count
// and highest rowid: a row added changes the count, and a row deleted and
// another added changes the rowid.
func (d *deletionTx) Fingerprint(ctx context.Context) (string, error) {
	tables := make([]string, 0, len(recordTables))
	for table := range recordTables {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	lines := make([]string, 0, len(tables))
	for _, table := range tables {
		var n int
		var maxRowid sql.NullInt64
		if err := d.tx.QueryRowContext(ctx, `SELECT count(*), max(rowid) FROM `+quoteIdent(table)).Scan(&n, &maxRowid); err != nil {
			return "", fmt.Errorf("failed to read %s: %w", table, err)
		}
		lines = append(lines, fmt.Sprintf("%s %d %d", table, n, maxRowid.Int64))
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

// DeleteAll deletes every record. The order does not matter: every
// foreign key cascades or detaches.
func (d *deletionTx) DeleteAll(ctx context.Context) error {
	for table := range recordTables {
		if _, err := d.tx.ExecContext(ctx, `DELETE FROM `+quoteIdent(table)); err != nil {
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
	}
	slog.InfoContext(ctx, "deleted all data")
	return nil
}

func (d *deletionTx) strings(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := d.tx.QueryContext(ctx, query, args...)
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

func chunked(ids []string, do func(chunk []string) error) error {
	for start := 0; start < len(ids); start += maxIDsPerStatement {
		end := min(start+maxIDsPerStatement, len(ids))
		if err := do(ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func anys(ids []string) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
