package llm

import (
	"errors"
	"fmt"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// Provider keys for known local LLM backends, used as domain.LLMSettings.Provider values.
const (
	ProviderOllama   = domain.LLMProviderOllama
	ProviderLlamaCpp = "llama.cpp"
	ProviderLMStudio = "lmstudio"
	ProviderCustom   = "custom"
)

// Preset is a known provider choice offered during settings configuration.
type Preset struct {
	Key            string
	Label          string
	DefaultBaseURL string
}

// Presets lists the known LLM backends offered when configuring settings,
// in display order. Custom is last and has no default base URL — the user
// must supply one, local or hosted.
//
// An OpenAI-compatible base URL includes its version, as every provider
// publishes it (ADR-0030 §4); Ollama's adapter is its own and takes the
// server's root.
var Presets = []Preset{
	{Key: ProviderOllama, Label: "Ollama", DefaultBaseURL: "http://localhost:11434"},
	{Key: ProviderLlamaCpp, Label: "llama.cpp (llama-server)", DefaultBaseURL: "http://localhost:8080/v1"},
	{Key: ProviderLMStudio, Label: "LM Studio", DefaultBaseURL: "http://localhost:1234/v1"},
	{Key: ProviderCustom, Label: "Custom (OpenAI-compatible endpoint)", DefaultBaseURL: ""},
}

// ResolveSettings fills in defaults for an unconfigured or partially
// configured LLMSettings: an empty Provider becomes ProviderOllama, and an
// empty BaseURL is filled from that provider's preset default (if any).
// This preserves JobPilot's original hardcoded behavior for anyone who has
// never configured settings.
func ResolveSettings(settings domain.LLMSettings) domain.LLMSettings {
	if settings.Provider == "" {
		settings.Provider = ProviderOllama
	}
	if settings.BaseURL == "" {
		for _, p := range Presets {
			if p.Key == settings.Provider {
				settings.BaseURL = p.DefaultBaseURL
				break
			}
		}
	}
	if settings.Model == "" && settings.Provider == ProviderOllama {
		settings.Model = DefaultModel
	}
	return settings
}

// NewProvider constructs the ports.LLMProvider described by settings, after
// resolving defaults via ResolveSettings. key is the secret stored under
// settings.APIKeyLabel, read by the caller; it is empty when the settings
// name none.
func NewProvider(settings domain.LLMSettings, key string) (ports.LLMProvider, error) {
	resolved := ResolveSettings(settings)
	if resolved.APIKeyLabel != "" && key == "" {
		return nil, fmt.Errorf("the API key %q was not given to the provider", resolved.APIKeyLabel)
	}

	switch resolved.Provider {
	case ProviderOllama:
		if resolved.APIKeyLabel != "" {
			return nil, errors.New("an API key is for an OpenAI-compatible endpoint; Ollama keeps its own sign-in")
		}
		return NewOllamaProvider(resolved.BaseURL, resolved.Model), nil
	case ProviderLlamaCpp, ProviderLMStudio, ProviderCustom:
		if resolved.BaseURL == "" {
			return nil, fmt.Errorf("base URL is required for provider %q", resolved.Provider)
		}
		return NewOpenAICompatProvider(resolved.Provider, resolved.BaseURL, resolved.Model).
			withKey(resolved.APIKeyLabel, key), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", resolved.Provider)
	}
}
