package llm

import (
	"fmt"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// Provider keys for known local LLM backends, used as domain.LLMSettings.Provider values.
const (
	ProviderOllama   = "ollama"
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

// Presets lists the known local LLM backends offered when configuring
// settings, in display order. Custom is last and has no default base URL —
// the user must supply one.
var Presets = []Preset{
	{Key: ProviderOllama, Label: "Ollama", DefaultBaseURL: "http://localhost:11434"},
	{Key: ProviderLlamaCpp, Label: "llama.cpp (llama-server)", DefaultBaseURL: "http://localhost:8080"},
	{Key: ProviderLMStudio, Label: "LM Studio", DefaultBaseURL: "http://localhost:1234"},
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
// resolving defaults via ResolveSettings.
func NewProvider(settings domain.LLMSettings) (ports.LLMProvider, error) {
	resolved := ResolveSettings(settings)

	switch resolved.Provider {
	case ProviderOllama:
		return NewOllamaProvider(resolved.BaseURL, resolved.Model), nil
	case ProviderLlamaCpp, ProviderLMStudio, ProviderCustom:
		if resolved.BaseURL == "" {
			return nil, fmt.Errorf("base URL is required for provider %q", resolved.Provider)
		}
		return NewOpenAICompatProvider(resolved.Provider, resolved.BaseURL, resolved.Model), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", resolved.Provider)
	}
}
