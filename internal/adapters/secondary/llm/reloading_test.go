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

// noKeys is a keychain with nothing in it.
func noKeys(context.Context, string) (string, error) {
	return "", ports.ErrCredentialNotFound
}

func TestReloadingProviderPicksUpSavedSettings(t *testing.T) {
	t.Parallel()

	stored := domain.LLMSettings{Provider: ProviderOllama, BaseURL: "http://localhost:11434", Model: "llama3"}
	provider := NewReloadingProvider(func(context.Context) (domain.LLMSettings, error) {
		return stored, nil
	}, noKeys)

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
	}, noKeys)

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
	}, noKeys)

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
	}, noKeys)

	err := provider.TestConnectivity(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown LLM provider")
}

// ADR-0030 §2: the key is read from the keychain on every call, so one
// replaced under the same label is used from the next request on.
func TestReloadingProviderReadsTheKeyEachTime(t *testing.T) {
	t.Parallel()

	settings := domain.LLMSettings{
		Provider: ProviderCustom, BaseURL: "https://api.openai.com/v1", Model: "gpt-5", APIKeyLabel: "openai",
	}
	stored := map[string]string{"openai": "sk-first"}
	var asked []string
	provider := NewReloadingProvider(
		func(context.Context) (domain.LLMSettings, error) { return settings, nil },
		func(_ context.Context, label string) (string, error) {
			asked = append(asked, label)
			key, ok := stored[label]
			if !ok {
				return "", ports.ErrCredentialNotFound
			}
			return key, nil
		},
	)

	first, err := provider.current(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "sk-first", first.(*OpenAICompatProvider).key.secret)
	again, err := provider.current(context.Background())
	require.NoError(t, err)
	assert.Same(t, first, again, "an unchanged key reuses the provider")

	stored["openai"] = "sk-second"
	replaced, err := provider.current(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "sk-second", replaced.(*OpenAICompatProvider).key.secret)
	assert.Equal(t, []string{"openai", "openai", "openai"}, asked, "read every time, by the label the settings name")

	delete(stored, "openai")
	err = provider.TestConnectivity(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `the AI model's API key "openai" is not stored`)
	assert.Contains(t, err.Error(), "Settings → AI model")
}

func TestReloadingProviderReadsNoKeyWhenNoneIsNamed(t *testing.T) {
	t.Parallel()

	provider := NewReloadingProvider(
		func(context.Context) (domain.LLMSettings, error) {
			return domain.LLMSettings{Provider: ProviderLMStudio, Model: "qwen"}, nil
		},
		func(context.Context, string) (string, error) {
			t.Fatal("the keychain is not asked for a key the settings do not name")
			return "", nil
		},
	)

	assert.Equal(t, ProviderLMStudio, provider.Provider())
}

func TestReloadingProviderReportsAKeychainFailure(t *testing.T) {
	t.Parallel()

	provider := NewReloadingProvider(
		func(context.Context) (domain.LLMSettings, error) {
			return domain.LLMSettings{
				Provider: ProviderCustom, BaseURL: "https://api.openai.com/v1", Model: "gpt-5", APIKeyLabel: "openai",
			}, nil
		},
		func(context.Context, string) (string, error) {
			return "", ports.ErrCredentialStoreUnavailable
		},
	)

	_, err := provider.Generate(context.Background(), ports.Request{})
	require.ErrorIs(t, err, ports.ErrCredentialStoreUnavailable)
	assert.Contains(t, err.Error(), `reading the AI model's API key "openai"`)
}
