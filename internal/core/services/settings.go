package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// llmSettingsKey is the user_settings key under which the configured LLM
// backend is stored as a JSON blob.
const llmSettingsKey = "llm_settings"

// SettingsService manages user-configurable local settings.
type SettingsService struct {
	repo ports.SettingsRepository
}

// NewSettingsService constructs a SettingsService backed by the given
// settings repository.
func NewSettingsService(repo ports.SettingsRepository) *SettingsService {
	return &SettingsService{repo: repo}
}

// GetLLMSettings returns the configured LLM backend. If none has been saved
// yet, it returns a zero-value LLMSettings (empty Provider) — callers treat
// an empty Provider as "use the built-in default."
func (s *SettingsService) GetLLMSettings(ctx context.Context) (*domain.LLMSettings, error) {
	value, found, err := s.repo.Get(ctx, llmSettingsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM settings: %w", err)
	}
	if !found {
		return &domain.LLMSettings{}, nil
	}

	var settings domain.LLMSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, fmt.Errorf("failed to parse stored LLM settings: %w", err)
	}

	return &settings, nil
}

// SaveLLMSettings validates and persists the configured LLM backend.
func (s *SettingsService) SaveLLMSettings(ctx context.Context, settings *domain.LLMSettings) error {
	if settings.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if settings.Model == "" {
		return fmt.Errorf("model is required")
	}

	parsed, err := url.ParseRequestURI(settings.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("base URL must be a valid absolute http(s) URL, got %q", settings.BaseURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("base URL must use http or https, got %q", settings.BaseURL)
	}

	value, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to encode LLM settings: %w", err)
	}

	if err := s.repo.Set(ctx, llmSettingsKey, string(value)); err != nil {
		return fmt.Errorf("failed to save LLM settings: %w", err)
	}

	return nil
}
