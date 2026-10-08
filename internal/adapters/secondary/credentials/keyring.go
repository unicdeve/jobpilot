// Package credentials stores secrets in OS-native secure storage: the
// macOS keychain, the Secret Service on Linux, and the Windows credential
// manager (ADR-0019).
package credentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// keychainService is the service name every JobPilot credential is filed
// under, so that a user can find them in their OS keychain and so
// DeleteAll-style tooling could not reach anything else.
const keychainService = "jobpilot"

// keychainTimeout bounds every keychain call.
//
// It exists because the failure that was measured is not an error. Running
// the daemon where no login keychain is available — no desktop session, a
// container, CI — makes /usr/bin/security wait for a password prompt that
// nobody will answer, and it waits forever: the HTTP request that triggered
// it never returns, and the user sees a hang with no message. The Linux
// Secret Service can do the same thing over D-Bus.
//
// Ten seconds is long enough that a keychain merely asking the user to
// approve access still succeeds, and short enough that an unattended run
// gets an answer.
const keychainTimeout = 10 * time.Second

// KeyringStore is a ports.CredentialStore over the OS keychain.
//
// An environment variable takes precedence over the keychain on read
// (ADR-0019 §5). That is what keeps the product usable where no keychain
// exists — headless Linux with no Secret Service, a container, CI — without
// inventing a second place a secret can be written.
type KeyringStore struct {
	// lookupEnv is os.LookupEnv except in tests.
	lookupEnv func(string) (string, bool)
	namespace string
}

// NewKeyringStore constructs a KeyringStore. A non-empty namespace isolates
// otherwise identical credential references belonging to another workspace.
func NewKeyringStore(namespace ...string) *KeyringStore {
	store := &KeyringStore{lookupEnv: os.LookupEnv}
	if len(namespace) > 0 {
		store.namespace = namespace[0]
	}
	return store
}

// Set stores a secret in the keychain.
//
// It never writes the environment: an override is read-only, so there is
// one place a credential is ever stored and no question about which copy
// won.
func (s *KeyringStore) Set(ctx context.Context, ref domain.CredentialRef, secret string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	// The error is wrapped with the ref and never the secret. The keychain
	// has a size ceiling — measured at 3009 bytes on macOS — so this can
	// genuinely fail on a large token and must not be assumed to succeed.
	err := s.withTimeout(ctx, ref, func() error {
		return keyring.Set(keychainService, s.account(ref), secret)
	})
	if err != nil {
		return fmt.Errorf("failed to store the credential for %s: %w", ref, err)
	}
	return nil
}

// Get returns the secret for ref, preferring an environment override.
func (s *KeyringStore) Get(ctx context.Context, ref domain.CredentialRef) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}

	if s.FromEnvironment(ref) {
		value, _ := s.lookupEnv(ref.EnvVarName())
		return value, nil
	}

	var secret string
	err := s.withTimeout(ctx, ref, func() error {
		var getErr error
		secret, getErr = keyring.Get(keychainService, s.account(ref))
		return getErr
	})
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return "", fmt.Errorf("%s: %w", ref, ports.ErrCredentialNotFound)
	case err != nil:
		return "", fmt.Errorf("failed to read the credential for %s: %w", ref, err)
	}
	return secret, nil
}

// Delete removes a stored secret. Deleting one that is not there succeeds.
//
// The stored copy is removed whether or not an environment variable also
// supplies the credential: it is JobPilot's to delete, and leaving it would
// bring the key back the moment the variable was unset. The variable itself
// cannot be removed, and Delete says so rather than reporting a success the
// user would find untrue the next time a connector ran.
func (s *KeyringStore) Delete(ctx context.Context, ref domain.CredentialRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}

	err := s.withTimeout(ctx, ref, func() error {
		return keyring.Delete(keychainService, s.account(ref))
	})
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("failed to delete the credential for %s: %w", ref, err)
	}

	if s.FromEnvironment(ref) {
		return fmt.Errorf(
			"the stored credential for %s was deleted, but the environment variable %s still supplies one, which JobPilot cannot unset — remove it there: %w",
			ref, ref.EnvVarName(), ports.ErrCredentialFromEnvironment)
	}
	return nil
}

// FromEnvironment reports whether ref's environment variable supplies it.
// An exported-but-empty variable does not, as on read.
func (s *KeyringStore) FromEnvironment(ref domain.CredentialRef) bool {
	value, ok := s.lookupEnv(ref.EnvVarName())
	return ok && value != ""
}

func (s *KeyringStore) account(ref domain.CredentialRef) string {
	if s.namespace == "" {
		return ref.String()
	}
	return s.namespace + "/" + ref.String()
}

// withTimeout runs a keychain call under ctx and keychainTimeout.
//
// The goroutine is left running if the call never returns: go-keyring owns
// the child process and exposes no way to kill it, so the choice is a
// leaked goroutine or a hung request. One goroutine per abandoned call is
// bounded by how many times the user retries; a hung request is not.
func (s *KeyringStore) withTimeout(ctx context.Context, ref domain.CredentialRef, call func() error) error {
	ctx, cancel := context.WithTimeout(ctx, keychainTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- call() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// The message names the environment variable that would work
		// instead, because the machines this happens on are the ones with
		// no way to answer a prompt.
		return fmt.Errorf(
			"%w after %s — if this machine has no desktop session, set %s instead: %w",
			ports.ErrCredentialStoreUnavailable, keychainTimeout, ref.EnvVarName(), ctx.Err())
	}
}
