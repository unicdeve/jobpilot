package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// CredentialService is how a credential is stored, listed and removed
// (ADR-0019).
//
// It spans two stores that no transaction covers: the OS keychain holds the
// secret, and a table records which refs exist because a keychain cannot be
// enumerated. The keychain is the truth and the table is a directory, which
// is what decides the order of every operation below.
type CredentialService struct {
	store ports.CredentialStore
	names ports.CredentialNameRepository
}

// NewCredentialService constructs a CredentialService.
func NewCredentialService(store ports.CredentialStore, names ports.CredentialNameRepository) *CredentialService {
	return &CredentialService{store: store, names: names}
}

// Set stores a secret and records that it exists.
//
// The keychain is written first. A crash between the two leaves a stored
// secret that nothing lists, which the user fixes by setting it again; the
// reverse order would leave a listed credential no connector can read,
// which looks like corruption and reads as configured in the UI.
//
// A failure to record is still an error, because a credential the product
// cannot list is one the user cannot manage.
func (s *CredentialService) Set(ctx context.Context, source, label, secret string) (domain.CredentialRef, error) {
	ref, err := domain.NewCredentialRef(source, label)
	if err != nil {
		return domain.CredentialRef{}, err
	}
	// The rule lives here rather than in the keystore, so that it holds for
	// every implementation of the port rather than for whichever one
	// remembered it — the same reason SearchCriteria filtering moved out of
	// the connectors and into JobDiscoveryService.
	if secret == "" {
		return domain.CredentialRef{}, fmt.Errorf("refusing to store an empty credential for %s", ref)
	}

	if err := s.store.Set(ctx, ref, secret); err != nil {
		return domain.CredentialRef{}, err
	}
	if err := s.names.Record(ctx, ref); err != nil {
		return domain.CredentialRef{}, err
	}
	return ref, nil
}

// List returns the refs that are configured, and never a value.
//
// It reads the directory rather than the keychain, so a ref recorded for a
// secret that has since been removed from the keychain by hand will be
// listed. Get is where that disagreement is resolved.
func (s *CredentialService) List(ctx context.Context) ([]domain.CredentialRef, error) {
	return s.names.List(ctx)
}

// Delete removes a secret and forgets the ref.
//
// The keychain goes first here too, for the same reason: the state to avoid
// is a ref the user can see and nothing can use.
func (s *CredentialService) Delete(ctx context.Context, source, label string) error {
	ref, err := domain.NewCredentialRef(source, label)
	if err != nil {
		return err
	}

	if err := s.store.Delete(ctx, ref); err != nil {
		return err
	}
	return s.names.Forget(ctx, ref)
}

// Get returns a stored secret, for use inside the daemon only.
//
// Nothing reachable from the API calls this. A credential goes in through
// the API and never comes out (ADR-0019 §4): the connector that needs a key
// runs in this process and reads it here.
//
// The keychain decides, whatever the directory says. A ref that is listed
// but absent from the keychain returns ErrCredentialNotFound, so a
// connector treats it as unconfigured rather than as a failure.
func (s *CredentialService) Get(ctx context.Context, source, label string) (string, error) {
	ref, err := domain.NewCredentialRef(source, label)
	if err != nil {
		return "", err
	}
	return s.store.Get(ctx, ref)
}

// Require returns a stored secret, or an error a user can act on.
//
// It is what a connector calls when it cannot work without a key, so the
// message names the source and says where to put one rather than reporting
// an absence the user has to interpret.
func (s *CredentialService) Require(ctx context.Context, source, label string) (string, error) {
	secret, err := s.Get(ctx, source, label)
	if errors.Is(err, ports.ErrCredentialNotFound) {
		return "", fmt.Errorf(
			"%s needs an API key, which is not configured — add one in Settings, or with 'jobpilot-cli credentials set %s'",
			source, source)
	}
	return secret, err
}
