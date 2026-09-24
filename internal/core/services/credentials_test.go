package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fakeCredentialStore is an in-memory ports.CredentialStore that records
// the order of calls, so the write ordering ADR-0019 §6 depends on can be
// asserted rather than assumed.
type fakeCredentialStore struct {
	secrets map[string]string
	calls   *[]string
	setErr  error
	delErr  error
}

func (f *fakeCredentialStore) Set(_ context.Context, ref domain.CredentialRef, secret string) error {
	*f.calls = append(*f.calls, "store.Set")
	if f.setErr != nil {
		return f.setErr
	}
	f.secrets[ref.String()] = secret
	return nil
}

func (f *fakeCredentialStore) Get(_ context.Context, ref domain.CredentialRef) (string, error) {
	*f.calls = append(*f.calls, "store.Get")
	secret, ok := f.secrets[ref.String()]
	if !ok {
		return "", ports.ErrCredentialNotFound
	}
	return secret, nil
}

func (f *fakeCredentialStore) Delete(_ context.Context, ref domain.CredentialRef) error {
	*f.calls = append(*f.calls, "store.Delete")
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.secrets, ref.String())
	return nil
}

type fakeCredentialNames struct {
	refs  []domain.CredentialRef
	calls *[]string
}

func (f *fakeCredentialNames) Record(_ context.Context, ref domain.CredentialRef) error {
	*f.calls = append(*f.calls, "names.Record")
	for _, existing := range f.refs {
		if existing == ref {
			return nil
		}
	}
	f.refs = append(f.refs, ref)
	return nil
}

func (f *fakeCredentialNames) Forget(_ context.Context, ref domain.CredentialRef) error {
	*f.calls = append(*f.calls, "names.Forget")
	kept := f.refs[:0]
	for _, existing := range f.refs {
		if existing != ref {
			kept = append(kept, existing)
		}
	}
	f.refs = kept
	return nil
}

func (f *fakeCredentialNames) List(_ context.Context) ([]domain.CredentialRef, error) {
	*f.calls = append(*f.calls, "names.List")
	return f.refs, nil
}

func newCredentialService() (*CredentialService, *fakeCredentialStore, *fakeCredentialNames, *[]string) {
	calls := &[]string{}
	store := &fakeCredentialStore{secrets: map[string]string{}, calls: calls}
	names := &fakeCredentialNames{calls: calls}
	return NewCredentialService(store, names), store, names, calls
}

func TestCredentialService_SetWritesTheKeychainFirst(t *testing.T) {
	t.Parallel()
	svc, _, _, calls := newCredentialService()

	ref, err := svc.Set(context.Background(), "adzuna", "work", "a-key")
	require.NoError(t, err)
	assert.Equal(t, "adzuna/work", ref.String())

	// The order is the whole point. A crash between the two must leave a
	// stored secret nothing lists — recoverable by setting it again —
	// rather than a listed credential no connector can read, which looks
	// like corruption and reads as configured in the UI.
	assert.Equal(t, []string{"store.Set", "names.Record"}, *calls)
}

func TestCredentialService_AFailedStoreRecordsNothing(t *testing.T) {
	t.Parallel()
	svc, store, names, _ := newCredentialService()
	store.setErr = errors.New("keychain unavailable")

	_, err := svc.Set(context.Background(), "adzuna", "", "a-key")

	require.Error(t, err)
	assert.Empty(t, names.refs, "a credential that was never stored must not be listed as configured")
}

func TestCredentialService_ListReturnsRefsAndNeverValues(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newCredentialService()
	ctx := context.Background()

	_, err := svc.Set(ctx, "adzuna", "work", "work-key")
	require.NoError(t, err)
	_, err = svc.Set(ctx, "adzuna", "personal", "personal-key")
	require.NoError(t, err)

	refs, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	assert.Equal(t, "adzuna/work", refs[0].String())
	assert.Equal(t, "adzuna/personal", refs[1].String())
}

func TestCredentialService_DeleteRemovesBoth(t *testing.T) {
	t.Parallel()
	svc, store, names, _ := newCredentialService()
	ctx := context.Background()

	_, err := svc.Set(ctx, "adzuna", "", "a-key")
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, "adzuna", ""))

	assert.Empty(t, store.secrets)
	assert.Empty(t, names.refs)
}

func TestCredentialService_AFailedDeleteKeepsTheRefListed(t *testing.T) {
	t.Parallel()
	svc, store, names, _ := newCredentialService()
	ctx := context.Background()

	_, err := svc.Set(ctx, "adzuna", "", "a-key")
	require.NoError(t, err)

	store.delErr = errors.New("the credential comes from the environment")
	require.Error(t, svc.Delete(ctx, "adzuna", ""))

	assert.Len(t, names.refs, 1,
		"a credential that is still readable must still be listed, or the user cannot see what a run will use")
}

// TestCredentialService_TheKeychainDecides pins the resolution of the one
// disagreement the two stores can have.
func TestCredentialService_TheKeychainDecides(t *testing.T) {
	t.Parallel()
	svc, store, names, _ := newCredentialService()
	ctx := context.Background()

	_, err := svc.Set(ctx, "adzuna", "", "a-key")
	require.NoError(t, err)

	// As if the user removed it from their keychain by hand.
	delete(store.secrets, "adzuna/default")

	listed, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, listed, 1, "the directory still says it exists")
	assert.Len(t, names.refs, 1)

	_, err = svc.Get(ctx, "adzuna", "")
	assert.ErrorIs(t, err, ports.ErrCredentialNotFound,
		"but a connector asking for it is told it is not configured")
}

func TestCredentialService_RequireExplainsWhatToDo(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newCredentialService()

	_, err := svc.Require(context.Background(), "adzuna", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "adzuna")
	assert.Contains(t, err.Error(), "credentials set",
		"a missing key should say where to put one, not just that it is absent")
}

func TestCredentialService_RefusesAnEmptySecret(t *testing.T) {
	t.Parallel()
	svc, store, names, _ := newCredentialService()

	_, err := svc.Set(context.Background(), "adzuna", "", "")

	require.ErrorContains(t, err, "empty")
	assert.Empty(t, store.secrets, "nothing should be stored")
	assert.Empty(t, names.refs, "and nothing listed")
}

func TestCredentialService_RejectsAMalformedRef(t *testing.T) {
	t.Parallel()
	svc, store, names, _ := newCredentialService()

	_, err := svc.Set(context.Background(), "Bad Source", "", "a-key")

	require.Error(t, err)
	assert.Empty(t, store.secrets)
	assert.Empty(t, names.refs)
}
