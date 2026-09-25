package ports

import (
	"context"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// DeletionStore is where deletions are carried out (ADR-0021).
//
// Core walks domain.DeletionRules to work out what a deletion reaches; the
// store answers the questions that walk asks of stored data, and deletes.
// Deleting a record's row is enough, because the schema's foreign keys
// carry the same rules and a test proves it.
type DeletionStore interface {
	// InTransaction runs do in one transaction, so a reach and the
	// deletion it permits see the same data. do's error rolls it back.
	InTransaction(ctx context.Context, do func(tx DeletionTx) error) error
	// Compact makes deleted content unreadable from the files on disk:
	// the WAL is checkpointed and truncated, and with full, the database is
	// also rewritten without its free pages.
	Compact(ctx context.Context, full bool) error
}

// DeletionTx is the stored data one deletion sees.
type DeletionTx interface {
	// Roots returns the IDs of the records target deletes directly, which
	// are of the returned kind. A target naming a record that does not
	// exist returns no IDs.
	Roots(ctx context.Context, target domain.DeletionTarget, id string) (domain.RecordKind, []string, error)
	// Referencing returns the IDs of the rule's referencing records that
	// reference any of ids.
	Referencing(ctx context.Context, rule domain.DeletionRule, ids []string) ([]string, error)
	// SentApplications counts the applications among ids that were sent.
	SentApplications(ctx context.Context, ids []string) (int, error)
	// KeptNotInterested counts the jobs "clear jobs" leaves because they
	// are marked not interested.
	KeptNotInterested(ctx context.Context) (int, error)
	// IsCurrentCoverLetter reports whether a cover letter is the one its
	// application uses.
	IsCurrentCoverLetter(ctx context.Context, id string) (bool, error)
	// Delete deletes the records of kind with ids.
	Delete(ctx context.Context, kind domain.RecordKind, ids []string) error

	// CountAll counts every stored record, by kind.
	CountAll(ctx context.Context) (map[domain.RecordKind]int, error)
	// Fingerprint identifies the database's current contents, for the
	// token of "delete all data", whose reach is everything.
	Fingerprint(ctx context.Context) (string, error)
	// DeleteAll deletes every record, keeping the schema.
	DeleteAll(ctx context.Context) error
}

// LocalDataEraser removes something JobPilot keeps outside the database,
// for "delete all data" (ADR-0021 §7).
type LocalDataEraser interface {
	// Describe names what Erase removes, for the confirmation.
	Describe() string
	// Erase removes it. Erasing what is already gone is not an error, so a
	// failed "delete all data" can be run again to finish.
	Erase(ctx context.Context) error
}
