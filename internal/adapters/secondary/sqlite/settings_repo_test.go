//go:build integration

package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsRepository_Get_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewSettingsRepository(db.Conn())

	value, found, err := repo.Get(context.Background(), "llm_settings")

	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, value)
}

func TestSettingsRepository_SetAndGet(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewSettingsRepository(db.Conn())

	require.NoError(t, repo.Set(context.Background(), "llm_settings", `{"provider":"ollama"}`))

	value, found, err := repo.Get(context.Background(), "llm_settings")

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, `{"provider":"ollama"}`, value)
}

func TestSettingsRepository_Set_OverwritesExistingValue(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := NewSettingsRepository(db.Conn())

	require.NoError(t, repo.Set(context.Background(), "llm_settings", "first"))
	require.NoError(t, repo.Set(context.Background(), "llm_settings", "second"))

	value, found, err := repo.Get(context.Background(), "llm_settings")

	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "second", value)
}
