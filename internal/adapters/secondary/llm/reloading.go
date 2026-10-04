package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// SettingsLoader reads the user's currently configured LLM backend.
type SettingsLoader func(ctx context.Context) (domain.LLMSettings, error)

// KeyLoader reads the secret stored under label for the model (ADR-0030
// §1). It answers ports.ErrCredentialNotFound for a label with nothing
// stored.
type KeyLoader func(ctx context.Context, label string) (string, error)

// ReloadingProvider is an ports.LLMProvider that resolves the configured
// backend at call time rather than at construction.
//
// The daemon outlives the desktop app and the CLI, so a provider built once
// during wiring would keep serving the settings that happened to be stored
// when it started: a user who configured their backend would keep getting
// failures from the old one until they found and restarted the daemon
// themselves. Resolving per call means saving settings takes effect on the
// next request.
//
// The settings name the API key by its label, and the key itself is read
// from the keychain on the same schedule, so a key replaced under the same
// label is used from the next request on (ADR-0030 §2).
//
// The constructed provider is cached and only rebuilt when the settings or
// the key actually change, so this costs one settings read, and one
// keychain read when there is a key, per generation.
type ReloadingProvider struct {
	load SettingsLoader
	keys KeyLoader

	mu          sync.Mutex
	cached      ports.LLMProvider
	cachedOn    domain.LLMSettings
	cachedOnKey string
}

// NewReloadingProvider constructs a ReloadingProvider that reads settings
// through load and API keys through keys.
func NewReloadingProvider(load SettingsLoader, keys KeyLoader) *ReloadingProvider {
	return &ReloadingProvider{load: load, keys: keys}
}

// current returns the provider described by the currently stored settings.
func (p *ReloadingProvider) current(ctx context.Context) (ports.LLMProvider, error) {
	settings, err := p.load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading LLM settings: %w", err)
	}
	resolved := ResolveSettings(settings)
	key, err := p.key(ctx, resolved)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached != nil && p.cachedOn == resolved && p.cachedOnKey == key {
		return p.cached, nil
	}

	provider, err := NewProvider(resolved, key)
	if err != nil {
		return nil, fmt.Errorf("configuring LLM provider: %w", err)
	}
	p.cached, p.cachedOn, p.cachedOnKey = provider, resolved, key
	return provider, nil
}

// key reads the API key the settings name, or none when they name none.
func (p *ReloadingProvider) key(ctx context.Context, settings domain.LLMSettings) (string, error) {
	if settings.APIKeyLabel == "" {
		return "", nil
	}
	key, err := p.keys(ctx, settings.APIKeyLabel)
	if errors.Is(err, ports.ErrCredentialNotFound) {
		return "", fmt.Errorf(
			"the AI model's API key %q is not stored — add it in Settings → AI model, or with 'jobpilot-cli settings set-llm'",
			settings.APIKeyLabel)
	}
	if err != nil {
		return "", fmt.Errorf("reading the AI model's API key %q: %w", settings.APIKeyLabel, err)
	}
	return key, nil
}

// Generate delegates to the currently configured backend.
func (p *ReloadingProvider) Generate(ctx context.Context, req ports.Request) (ports.Response, error) {
	provider, err := p.current(ctx)
	if err != nil {
		return ports.Response{}, err
	}
	return provider.Generate(ctx, req)
}

// TestConnectivity delegates to the currently configured backend.
func (p *ReloadingProvider) TestConnectivity(ctx context.Context) error {
	provider, err := p.current(ctx)
	if err != nil {
		return err
	}
	return provider.TestConnectivity(ctx)
}

// Provider returns the identifier of the currently configured backend. It is
// only used to label generated results, and has nowhere to report a failure
// to read settings, so it falls back to the last backend that resolved.
func (p *ReloadingProvider) Provider() string {
	if provider, err := p.current(context.Background()); err == nil {
		return provider.Provider()
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached != nil {
		return p.cached.Provider()
	}
	return ""
}
