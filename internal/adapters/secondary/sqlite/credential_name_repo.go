package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// CredentialNameRepository is a ports.CredentialNameRepository backed by
// SQLite.
//
// It stores which credentials exist and never their values: the secret is
// in the OS keychain, and this table is the directory the keychain cannot
// provide (ADR-0019 §6).
type CredentialNameRepository struct {
	db *sql.DB
}

// NewCredentialNameRepository constructs a CredentialNameRepository over db.
func NewCredentialNameRepository(db *sql.DB) *CredentialNameRepository {
	return &CredentialNameRepository{db: db}
}

// Record notes that a credential exists for ref, refreshing updated_at when
// one is already recorded.
func (r *CredentialNameRepository) Record(ctx context.Context, ref domain.CredentialRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}

	const query = `INSERT INTO credential_names (source, label) VALUES (?, ?)
		ON CONFLICT(source, label) DO UPDATE SET updated_at = CURRENT_TIMESTAMP`

	if _, err := r.db.ExecContext(ctx, query, ref.Source, ref.Label); err != nil {
		return fmt.Errorf("failed to record the credential %s: %w", ref, err)
	}
	return nil
}

// Forget removes a ref from the directory. Forgetting one that is not
// recorded is not an error.
func (r *CredentialNameRepository) Forget(ctx context.Context, ref domain.CredentialRef) error {
	const query = `DELETE FROM credential_names WHERE source = ? AND label = ?`

	if _, err := r.db.ExecContext(ctx, query, ref.Source, ref.Label); err != nil {
		return fmt.Errorf("failed to forget the credential %s: %w", ref, err)
	}
	return nil
}

// List returns every recorded ref, by source then label.
func (r *CredentialNameRepository) List(ctx context.Context) ([]domain.CredentialRef, error) {
	const query = `SELECT source, label FROM credential_names ORDER BY source, label`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	refs := make([]domain.CredentialRef, 0)
	for rows.Next() {
		var ref domain.CredentialRef
		if err := rows.Scan(&ref.Source, &ref.Label); err != nil {
			return nil, fmt.Errorf("failed to read a credential row: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the credential list: %w", err)
	}
	return refs, nil
}
