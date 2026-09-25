package ports

import (
	"context"
	"errors"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// ErrCredentialNotFound is returned by CredentialStore.Get when nothing is
// stored for a ref. It is a named error because "not configured" is an
// ordinary state a caller acts on — a connector without a key is skipped,
// not failed — and must be distinguishable from a keychain that could not
// be reached at all.
var ErrCredentialNotFound = errors.New("no credential stored")

// ErrCredentialStoreUnavailable is returned when the keystore neither
// answered nor refused: a keychain with no desktop session behind it waits
// for a prompt nobody will answer, measured on macOS and expected of the
// Linux Secret Service too.
//
// It is distinct from ErrCredentialNotFound because the two are opposite
// answers to "can I use this credential", and distinct from a validation
// error because it is not the caller's mistake — an API that reported it as
// one would tell the user to fix a request that was correct.
var ErrCredentialStoreUnavailable = errors.New("the credential store did not respond")

// ErrCredentialFromEnvironment is returned by CredentialStore.Delete when,
// having removed the stored copy, the credential is still supplied by its
// environment variable (ADR-0019 §5), which JobPilot cannot unset. It is a
// named error so "delete all data" can report it rather than stop at it.
var ErrCredentialFromEnvironment = errors.New("an environment variable still supplies this credential")

// CredentialStore holds API keys and other secrets outside the application
// database, in OS-native secure storage (ADR-0019, and the "Secret storage"
// rule in docs/01_architecture/08_security.md).
//
// It is a port so that core never chooses a keystore, and so a service that
// needs a credential is testable without one.
//
// There is deliberately no List. The keychain cannot be enumerated, and a
// user-chosen label cannot be guessed, so which credentials exist is
// answered by a separate index (ADR-0019 §6) rather than by this interface
// pretending it can.
type CredentialStore interface {
	// Set stores a secret, replacing any already held for the ref.
	Set(ctx context.Context, ref domain.CredentialRef, secret string) error
	// Get returns the stored secret, or ErrCredentialNotFound.
	//
	// Nothing outside the daemon may call this: a credential goes in
	// through the API and never comes out (ADR-0019 §4).
	Get(ctx context.Context, ref domain.CredentialRef) (string, error)
	// Delete removes a stored secret. Deleting one that is not there is
	// not an error, so that a caller cleaning up need not check first.
	// When the ref's environment variable also supplies it, the stored
	// copy is removed and ErrCredentialFromEnvironment returned.
	Delete(ctx context.Context, ref domain.CredentialRef) error
	// FromEnvironment reports whether the ref's environment variable
	// supplies it, which no deletion can change.
	FromEnvironment(ref domain.CredentialRef) bool
}
