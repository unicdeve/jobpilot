package credentials

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// newTestStore returns a store over go-keyring's in-memory mock, so these
// tests never touch the developer's real keychain.
func newTestStore(env map[string]string) *KeyringStore {
	keyring.MockInit()
	return &KeyringStore{
		lookupEnv: func(name string) (string, bool) {
			value, ok := env[name]
			return value, ok
		},
	}
}

func ref(t *testing.T, source, label string) domain.CredentialRef {
	t.Helper()
	r, err := domain.NewCredentialRef(source, label)
	require.NoError(t, err)
	return r
}

func TestKeyringStore_RoundTrip(t *testing.T) {
	store := newTestStore(nil)
	ctx := context.Background()
	adzuna := ref(t, "adzuna", "default")

	require.NoError(t, store.Set(ctx, adzuna, "key-one"))

	got, err := store.Get(ctx, adzuna)
	require.NoError(t, err)
	assert.Equal(t, "key-one", got)

	// Setting again replaces rather than duplicating.
	require.NoError(t, store.Set(ctx, adzuna, "key-two"))
	got, err = store.Get(ctx, adzuna)
	require.NoError(t, err)
	assert.Equal(t, "key-two", got)

	require.NoError(t, store.Delete(ctx, adzuna))
	_, err = store.Get(ctx, adzuna)
	assert.ErrorIs(t, err, ports.ErrCredentialNotFound)
}

func TestKeyringStore_LabelsAreSeparateCredentials(t *testing.T) {
	store := newTestStore(nil)
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, ref(t, "adzuna", "work"), "work-key"))
	require.NoError(t, store.Set(ctx, ref(t, "adzuna", "personal"), "personal-key"))

	work, err := store.Get(ctx, ref(t, "adzuna", "work"))
	require.NoError(t, err)
	personal, err := store.Get(ctx, ref(t, "adzuna", "personal"))
	require.NoError(t, err)

	assert.Equal(t, "work-key", work)
	assert.Equal(t, "personal-key", personal)

	// Deleting one leaves the other.
	require.NoError(t, store.Delete(ctx, ref(t, "adzuna", "work")))
	personal, err = store.Get(ctx, ref(t, "adzuna", "personal"))
	require.NoError(t, err)
	assert.Equal(t, "personal-key", personal)
}

func TestKeyringStore_NotFoundIsNotAFailure(t *testing.T) {
	store := newTestStore(nil)
	ctx := context.Background()

	_, err := store.Get(ctx, ref(t, "adzuna", "default"))
	assert.ErrorIs(t, err, ports.ErrCredentialNotFound,
		"a connector must be able to tell 'not configured' from 'the keychain broke'")

	// Deleting one that was never stored is not an error, so a caller
	// cleaning up need not check first.
	assert.NoError(t, store.Delete(ctx, ref(t, "adzuna", "default")))
}

func TestKeyringStore_EnvironmentOverrides(t *testing.T) {
	ctx := context.Background()
	adzuna := ref(t, "adzuna", "default")

	store := newTestStore(map[string]string{adzuna.EnvVarName(): "from-the-environment"})
	require.NoError(t, store.Set(ctx, adzuna, "from-the-keychain"))

	got, err := store.Get(ctx, adzuna)
	require.NoError(t, err)
	assert.Equal(t, "from-the-environment", got, "the override wins on read")

	// An override cannot be unset by JobPilot, so Delete says so rather
	// than reporting a success the next connector run would disprove.
	err = store.Delete(ctx, adzuna)
	require.Error(t, err)
	assert.Contains(t, err.Error(), adzuna.EnvVarName())
}

func TestKeyringStore_AnEmptyOverrideIsNotAnOverride(t *testing.T) {
	ctx := context.Background()
	adzuna := ref(t, "adzuna", "default")

	store := newTestStore(map[string]string{adzuna.EnvVarName(): ""})
	require.NoError(t, store.Set(ctx, adzuna, "from-the-keychain"))

	got, err := store.Get(ctx, adzuna)
	require.NoError(t, err)
	assert.Equal(t, "from-the-keychain", got,
		"an exported-but-empty variable is how a shell spells 'unset', not 'use nothing'")
}

// TestNoErrorCarriesTheSecret is ADR-0019 §7 as a test rather than a
// convention. The natural way to write a failure path —
// fmt.Errorf("storing %q: %w", secret, err) — is one character from
// correct, and the result would put an API key into a log the user then
// pastes into a bug report.
func TestNoErrorCarriesTheSecret(t *testing.T) {
	const secret = "sk-live-THIS-MUST-NEVER-APPEAR-anywhere"
	ctx := context.Background()
	adzuna := ref(t, "adzuna", "default")

	// A store whose keychain fails every call, so every error path runs.
	keyring.MockInitWithError(errors.New("keychain unavailable"))
	broken := &KeyringStore{lookupEnv: func(string) (string, bool) { return "", false }}

	var errs []error
	errs = append(errs, broken.Set(ctx, adzuna, secret))
	_, getErr := broken.Get(ctx, adzuna)
	errs = append(errs, getErr)
	errs = append(errs, broken.Delete(ctx, adzuna))

	// And the ordinary paths, where a secret is actually in hand.
	store := newTestStore(nil)
	require.NoError(t, store.Set(ctx, adzuna, secret))
	errs = append(errs, store.Set(ctx, domain.CredentialRef{Source: "BAD", Label: "x"}, secret))

	failed := 0
	for _, err := range errs {
		if err == nil {
			continue
		}
		failed++
		assert.NotContains(t, err.Error(), secret, "an error carried the secret")
		// Not even a prefix: the first characters of an API key identify
		// the account to anyone who has seen one before.
		assert.NotContains(t, err.Error(), secret[:8], "an error carried part of the secret")
	}

	// Without this the loop above passes by examining nothing.
	assert.Equal(t, len(errs), failed, "every call here should have failed, so every error path is checked")
}

// TestAHangingKeychainTimesOut covers the failure that was measured rather
// than guessed at.
//
// Running the daemon with no login keychain available does not produce an
// error: /usr/bin/security waits for a password prompt nobody will answer,
// and waits forever. Without a bound the HTTP request that triggered it
// never returns and the user sees a hang with no message.
func TestAHangingKeychainTimesOut(t *testing.T) {
	store := newTestStore(nil)
	adzuna := ref(t, "adzuna", "default")

	// A context already past its deadline stands in for a keychain that
	// will never answer, without making the test wait for the real bound.
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()

	err := store.withTimeout(ctx, adzuna, func() error {
		<-make(chan struct{}) // never returns
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ports.ErrCredentialStoreUnavailable)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), adzuna.EnvVarName(),
		"the machines this happens on are the ones that need the environment variable")
}

func TestATimeoutDoesNotStopAKeychainThatAnswers(t *testing.T) {
	store := newTestStore(nil)
	ctx := context.Background()
	adzuna := ref(t, "adzuna", "default")

	// The ordinary path still runs under the same wrapper.
	require.NoError(t, store.Set(ctx, adzuna, "a-key"))
	got, err := store.Get(ctx, adzuna)
	require.NoError(t, err)
	assert.Equal(t, "a-key", got)
}

func TestKeyringStore_RejectsAnInvalidRefBeforeTouchingTheKeychain(t *testing.T) {
	store := newTestStore(nil)
	ctx := context.Background()
	bad := domain.CredentialRef{Source: "has/slash", Label: "default"}

	assert.Error(t, store.Set(ctx, bad, "x"))
	_, err := store.Get(ctx, bad)
	assert.Error(t, err)
	assert.Error(t, store.Delete(ctx, bad))

	// The ref is named so the user can fix it; it holds no secret.
	assert.True(t, strings.Contains(err.Error(), "source"))
}
