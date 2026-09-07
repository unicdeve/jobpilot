package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestResolveSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   domain.LLMSettings
		want domain.LLMSettings
	}{
		{
			name: "empty settings default to ollama with its preset URL and model",
			in:   domain.LLMSettings{},
			want: domain.LLMSettings{Provider: ProviderOllama, BaseURL: "http://localhost:11434", Model: DefaultModel},
		},
		{
			name: "explicit values are left alone",
			in:   domain.LLMSettings{Provider: ProviderOllama, BaseURL: "http://example.com:1234", Model: "custom-model"},
			want: domain.LLMSettings{Provider: ProviderOllama, BaseURL: "http://example.com:1234", Model: "custom-model"},
		},
		{
			name: "llama.cpp with no base URL gets its preset default",
			in:   domain.LLMSettings{Provider: ProviderLlamaCpp},
			want: domain.LLMSettings{Provider: ProviderLlamaCpp, BaseURL: "http://localhost:8080"},
		},
		{
			name: "lmstudio with no base URL gets its preset default",
			in:   domain.LLMSettings{Provider: ProviderLMStudio},
			want: domain.LLMSettings{Provider: ProviderLMStudio, BaseURL: "http://localhost:1234"},
		},
		{
			name: "custom with no base URL stays empty, no ollama model default applied",
			in:   domain.LLMSettings{Provider: ProviderCustom},
			want: domain.LLMSettings{Provider: ProviderCustom, BaseURL: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ResolveSettings(tt.in))
		})
	}
}

func TestNewProvider(t *testing.T) {
	t.Parallel()

	t.Run("ollama", func(t *testing.T) {
		t.Parallel()
		p, err := NewProvider(domain.LLMSettings{Provider: ProviderOllama, BaseURL: "http://localhost:11434", Model: "llama3"})
		require.NoError(t, err)
		assert.IsType(t, &OllamaProvider{}, p)
		assert.Equal(t, "ollama", p.Provider())
	})

	t.Run("empty provider defaults to ollama", func(t *testing.T) {
		t.Parallel()
		p, err := NewProvider(domain.LLMSettings{})
		require.NoError(t, err)
		assert.IsType(t, &OllamaProvider{}, p)
	})

	for _, provider := range []string{ProviderLlamaCpp, ProviderLMStudio, ProviderCustom} {
		t.Run(provider+" with base URL", func(t *testing.T) {
			t.Parallel()
			p, err := NewProvider(domain.LLMSettings{Provider: provider, BaseURL: "http://localhost:9999", Model: "some-model"})
			require.NoError(t, err)
			assert.IsType(t, &OpenAICompatProvider{}, p)
			assert.Equal(t, provider, p.Provider())
		})
	}

	t.Run("custom with no base URL errors", func(t *testing.T) {
		t.Parallel()
		_, err := NewProvider(domain.LLMSettings{Provider: ProviderCustom})
		assert.ErrorContains(t, err, "base URL is required")
	})

	t.Run("unknown provider errors", func(t *testing.T) {
		t.Parallel()
		_, err := NewProvider(domain.LLMSettings{Provider: "not-a-real-provider"})
		assert.ErrorContains(t, err, "unknown LLM provider")
	})
}
