package credentials

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiveKeychain runs against the real OS keychain. It is skipped unless
// JOBPILOT_LIVE_KEYCHAIN is set, so an ordinary test run never writes to a
// developer's keychain.
func TestLiveKeychain(t *testing.T) {
	if os.Getenv("JOBPILOT_LIVE_KEYCHAIN") == "" {
		t.Skip("set JOBPILOT_LIVE_KEYCHAIN to run against the real keychain")
	}

	store := NewKeyringStore()
	ctx := context.Background()
	work := ref(t, "jobpilot-live-probe", "work")
	personal := ref(t, "jobpilot-live-probe", "personal")

	t.Cleanup(func() {
		_ = store.Delete(ctx, work)
		_ = store.Delete(ctx, personal)
	})

	require.NoError(t, store.Set(ctx, work, "work-key-123"))
	require.NoError(t, store.Set(ctx, personal, "personal-key-456"))

	got, err := store.Get(ctx, work)
	require.NoError(t, err)
	assert.Equal(t, "work-key-123", got)

	got, err = store.Get(ctx, personal)
	require.NoError(t, err)
	assert.Equal(t, "personal-key-456", got, "two labels are two credentials")

	require.NoError(t, store.Set(ctx, work, "replaced"))
	got, err = store.Get(ctx, work)
	require.NoError(t, err)
	assert.Equal(t, "replaced", got)

	require.NoError(t, store.Delete(ctx, work))
	_, err = store.Get(ctx, work)
	assert.Error(t, err)

	got, err = store.Get(ctx, personal)
	require.NoError(t, err)
	assert.Equal(t, "personal-key-456", got, "deleting one leaves the other")
}
