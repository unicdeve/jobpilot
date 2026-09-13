package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func TestReloadingProviderPicksUpSavedSettings(t *testing.T) {
	t.Parallel()

	stored := domain.LLMSettings{Provider: ProviderOllama, BaseURL: "http://localhost:11434", Model: "llama3"}
	provider := NewReloadingProvider(func(context.Context) (domain.LLMSettings, error) {
		return stored, nil
	})

	assert.Equal(t, ProviderOllama, provider.Provider())

	// The daemon is long-lived: settings saved after it started must take
	// effect without a restart.
	stored = domain.LLMSettings{Provider: ProviderLMStudio, BaseURL: "http://localhost:1234", Model: "qwen"}
	assert.Equal(t, ProviderLMStudio, provider.Provider())
}

func TestReloadingProviderReusesProviderWhileSettingsAreUnchanged(t *testing.T) {
	t.Parallel()

	settings := domain.LLMSettings{Provider: ProviderOllama, BaseURL: "http://localhost:11434", Model: "llama3"}
	provider := NewReloadingProvider(func(context.Context) (domain.LLMSettings, error) {
		return settings, nil
	})

	first, err := provider.current(context.Background())
	require.NoError(t, err)
	second, err := provider.current(context.Background())
	require.NoError(t, err)
	assert.Same(t, first, second)

	settings.Model = "mistral"
	third, err := provider.current(context.Background())
	require.NoError(t, err)
	assert.NotSame(t, first, third, "a model change must rebuild the provider")
}

func TestReloadingProviderReportsSettingsFailures(t *testing.T) {
	t.Parallel()

	provider := NewReloadingProvider(func(context.Context) (domain.LLMSettings, error) {
		return domain.LLMSettings{}, errors.New("database is locked")
	})

	_, err := provider.Generate(context.Background(), ports.Request{UserPrompt: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is locked")

	// Provider() has nowhere to report the failure and nothing cached yet.
	assert.Empty(t, provider.Provider())
}

func TestReloadingProviderRejectsUnknownProvider(t *testing.T) {
	t.Parallel()

	provider := NewReloadingProvider(func(context.Context) (domain.LLMSettings, error) {
		return domain.LLMSettings{Provider: "nonsense", BaseURL: "http://localhost:1", Model: "m"}, nil
	})

	err := provider.TestConnectivity(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown LLM provider")
}
