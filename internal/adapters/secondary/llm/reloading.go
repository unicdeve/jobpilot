package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// SettingsLoader reads the user's currently configured LLM backend.
type SettingsLoader func(ctx context.Context) (domain.LLMSettings, error)

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
// The constructed provider is cached and only rebuilt when the settings
// actually change, so this costs one settings read per generation.
type ReloadingProvider struct {
	load SettingsLoader

	mu       sync.Mutex
	cached   ports.LLMProvider
	cachedOn domain.LLMSettings
}

// NewReloadingProvider constructs a ReloadingProvider that reads settings
// through load.
func NewReloadingProvider(load SettingsLoader) *ReloadingProvider {
	return &ReloadingProvider{load: load}
}

// current returns the provider described by the currently stored settings.
func (p *ReloadingProvider) current(ctx context.Context) (ports.LLMProvider, error) {
	settings, err := p.load(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading LLM settings: %w", err)
	}
	resolved := ResolveSettings(settings)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached != nil && p.cachedOn == resolved {
		return p.cached, nil
	}

	provider, err := NewProvider(resolved)
	if err != nil {
		return nil, fmt.Errorf("configuring LLM provider: %w", err)
	}
	p.cached, p.cachedOn = provider, resolved
	return provider, nil
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
