//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration00024_VersionOpenAICompatBaseURL keeps every stored
// OpenAI-compatible backend pointed where it was, now that the adapter
// appends /chat/completions rather than /v1/chat/completions (ADR-0030 §4).
func TestMigration00024_VersionOpenAICompatBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stored string
		want   string
	}{
		{
			name:   "a local llama-server gains /v1",
			stored: `{"provider":"llama.cpp","base_url":"http://localhost:8080","model":"qwen"}`,
			want:   `{"base_url":"http://localhost:8080/v1","model":"qwen","provider":"llama.cpp"}`,
		},
		{
			name:   "a trailing slash is not doubled",
			stored: `{"provider":"lmstudio","base_url":"http://localhost:1234/","model":"qwen"}`,
			want:   `{"base_url":"http://localhost:1234/v1","model":"qwen","provider":"lmstudio"}`,
		},
		{
			name:   "a custom endpoint on a path gains /v1 after it",
			stored: `{"provider":"custom","base_url":"http://gpu-box:8000/openai","model":"m"}`,
			want:   `{"base_url":"http://gpu-box:8000/openai/v1","model":"m","provider":"custom"}`,
		},
		{
			name:   "one already ending in /v1 was failing, and is left to work",
			stored: `{"provider":"custom","base_url":"http://localhost:8000/v1","model":"m"}`,
			want:   `{"provider":"custom","base_url":"http://localhost:8000/v1","model":"m"}`,
		},
		{
			name:   "Ollama is untouched",
			stored: `{"provider":"ollama","base_url":"http://localhost:11434","model":"llama3"}`,
			want:   `{"provider":"ollama","base_url":"http://localhost:11434","model":"llama3"}`,
		},
		{
			name:   "an empty URL is left to its preset's default",
			stored: `{"provider":"llama.cpp","base_url":"","model":"qwen"}`,
			want:   `{"provider":"llama.cpp","base_url":"","model":"qwen"}`,
		},
		{
			name:   "a field the migration does not know is kept",
			stored: `{"provider":"llama.cpp","base_url":"http://localhost:8080","model":"qwen","extra":{"a":1}}`,
			want:   `{"base_url":"http://localhost:8080/v1","extra":{"a":1},"model":"qwen","provider":"llama.cpp"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := migratedTo(t, 23)
			ctx := context.Background()
			_, err := db.Conn().ExecContext(ctx,
				`INSERT INTO user_settings (key, value) VALUES ('llm_settings', ?)`, tt.stored)
			require.NoError(t, err)

			migrateUpTo(t, db, 24)

			var got string
			require.NoError(t, db.Conn().QueryRowContext(ctx,
				`SELECT value FROM user_settings WHERE key = 'llm_settings'`).Scan(&got))
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("nothing configured stays that way", func(t *testing.T) {
		t.Parallel()
		db := migratedTo(t, 23)

		migrateUpTo(t, db, 24)

		var got string
		err := db.Conn().QueryRowContext(context.Background(),
			`SELECT value FROM user_settings WHERE key = 'llm_settings'`).Scan(&got)
		assert.True(t, errors.Is(err, sql.ErrNoRows))
	})
}
