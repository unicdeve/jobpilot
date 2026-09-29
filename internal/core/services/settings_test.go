package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestSettingsService_GetLLMSettings(t *testing.T) {
	t.Parallel()

	t.Run("not found returns zero value", func(t *testing.T) {
		t.Parallel()
		svc := NewSettingsService(newFakeSettingsRepository())

		got, err := svc.GetLLMSettings(context.Background())

		require.NoError(t, err)
		assert.Equal(t, &domain.LLMSettings{}, got)
	})

	t.Run("found unmarshals stored value", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.values[llmSettingsKey] = `{"provider":"ollama","base_url":"http://localhost:11434","model":"llama3"}`

		svc := NewSettingsService(repo)
		got, err := svc.GetLLMSettings(context.Background())

		require.NoError(t, err)
		assert.Equal(t, &domain.LLMSettings{Provider: "ollama", BaseURL: "http://localhost:11434", Model: "llama3"}, got)
	})

	t.Run("corrupt stored value errors", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.values[llmSettingsKey] = "not json"

		svc := NewSettingsService(repo)
		_, err := svc.GetLLMSettings(context.Background())

		assert.ErrorContains(t, err, "failed to parse stored LLM settings")
	})

	t.Run("repo error is wrapped", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.getErr = errors.New("boom")

		svc := NewSettingsService(repo)
		_, err := svc.GetLLMSettings(context.Background())

		assert.ErrorContains(t, err, "failed to get LLM settings")
	})
}

func TestSettingsService_SaveLLMSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      domain.LLMSettings
		wantErr string
	}{
		{
			name:    "missing provider",
			in:      domain.LLMSettings{Model: "llama3", BaseURL: "http://localhost:11434"},
			wantErr: "provider is required",
		},
		{
			name:    "missing model",
			in:      domain.LLMSettings{Provider: "ollama", BaseURL: "http://localhost:11434"},
			wantErr: "model is required",
		},
		{
			name:    "missing base URL",
			in:      domain.LLMSettings{Provider: "ollama", Model: "llama3"},
			wantErr: "base URL must be a valid absolute http(s) URL",
		},
		{
			name:    "malformed base URL",
			in:      domain.LLMSettings{Provider: "ollama", Model: "llama3", BaseURL: "not a url"},
			wantErr: "base URL must be a valid absolute http(s) URL",
		},
		{
			name:    "non-http(s) scheme",
			in:      domain.LLMSettings{Provider: "ollama", Model: "llama3", BaseURL: "ftp://localhost:11434"},
			wantErr: "base URL must use http or https",
		},
		{
			name: "valid settings",
			in:   domain.LLMSettings{Provider: "ollama", Model: "llama3", BaseURL: "http://localhost:11434"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeSettingsRepository()
			svc := NewSettingsService(repo)

			err := svc.SaveLLMSettings(context.Background(), &tt.in)

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, repo.values)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, repo.values[llmSettingsKey], `"provider":"ollama"`)
		})
	}

	t.Run("repo error is wrapped", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.setErr = errors.New("boom")

		svc := NewSettingsService(repo)
		err := svc.SaveLLMSettings(context.Background(), &domain.LLMSettings{Provider: "ollama", Model: "llama3", BaseURL: "http://localhost:11434"})

		assert.ErrorContains(t, err, "failed to save LLM settings")
	})
}

func TestSettingsService_DiscoverySettings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewSettingsService(newFakeSettingsRepository())

	got, err := svc.GetDiscoverySettings(ctx)
	require.NoError(t, err)
	assert.True(t, got.RefreshOnLaunch, "refreshing on launch is on until turned off")

	require.NoError(t, svc.SaveDiscoverySettings(ctx, &domain.DiscoverySettings{RefreshOnLaunch: false}))
	got, err = svc.GetDiscoverySettings(ctx)
	require.NoError(t, err)
	assert.False(t, got.RefreshOnLaunch, "turned off stays off")
}
