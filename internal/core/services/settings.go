package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// llmSettingsKey is the user_settings key under which the configured LLM
// backend is stored as a JSON blob.
const llmSettingsKey = "llm_settings"

// discoverySettingsKey is the user_settings key for DiscoverySettings.
const discoverySettingsKey = "discovery_settings"

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
	if err := settings.Validate(); err != nil {
		return err
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

// GetDiscoverySettings returns how discovery behaves without being asked.
// Nothing saved yet is the defaults: refreshing on launch is on.
func (s *SettingsService) GetDiscoverySettings(ctx context.Context) (*domain.DiscoverySettings, error) {
	value, found, err := s.repo.Get(ctx, discoverySettingsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get discovery settings: %w", err)
	}
	if !found {
		return &domain.DiscoverySettings{RefreshOnLaunch: true}, nil
	}

	var settings domain.DiscoverySettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, fmt.Errorf("failed to parse stored discovery settings: %w", err)
	}
	return &settings, nil
}

// SaveDiscoverySettings persists how discovery behaves without being asked.
func (s *SettingsService) SaveDiscoverySettings(ctx context.Context, settings *domain.DiscoverySettings) error {
	value, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to encode discovery settings: %w", err)
	}
	if err := s.repo.Set(ctx, discoverySettingsKey, string(value)); err != nil {
		return fmt.Errorf("failed to save discovery settings: %w", err)
	}
	return nil
}

// notificationSettingsKey is the user_settings key for NotificationSettings.
const notificationSettingsKey = "notification_settings"

// desktopExecutableKey is the user_settings key for where the desktop
// app's executable is, as the app last said (ADR-0028 §1).
const desktopExecutableKey = "desktop_executable"

// GetNotificationSettings returns the user's choices about notifications,
// or the defaults when they have made none (ADR-0028 §6).
func (s *SettingsService) GetNotificationSettings(ctx context.Context) (*domain.NotificationSettings, error) {
	value, found, err := s.repo.Get(ctx, notificationSettingsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification settings: %w", err)
	}
	settings := domain.DefaultNotificationSettings()
	if !found {
		return &settings, nil
	}
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, fmt.Errorf("failed to parse stored notification settings: %w", err)
	}
	return &settings, nil
}

// SaveNotificationSettings validates and persists the user's choices about
// notifications.
func (s *SettingsService) SaveNotificationSettings(ctx context.Context, settings *domain.NotificationSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	value, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to encode notification settings: %w", err)
	}
	if err := s.repo.Set(ctx, notificationSettingsKey, string(value)); err != nil {
		return fmt.Errorf("failed to save notification settings: %w", err)
	}
	return nil
}

// GetDesktopExecutable returns where the desktop app's executable is, as
// the app last said, or "" if it never has.
func (s *SettingsService) GetDesktopExecutable(ctx context.Context) (string, error) {
	value, _, err := s.repo.Get(ctx, desktopExecutableKey)
	if err != nil {
		return "", fmt.Errorf("failed to get the desktop app's location: %w", err)
	}
	return value, nil
}

// SaveDesktopExecutable records where the desktop app's executable is. The
// app says so each time it starts, so a moved or rebuilt app is found.
func (s *SettingsService) SaveDesktopExecutable(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("the desktop app's executable must be an absolute path, got %q", path)
	}
	if err := s.repo.Set(ctx, desktopExecutableKey, path); err != nil {
		return fmt.Errorf("failed to save the desktop app's location: %w", err)
	}
	return nil
}

// promptGuidanceKey is the user_settings key for PromptGuidanceSettings
// (ADR-0031 §5).
const promptGuidanceKey = "prompt_guidance"

// ListPromptGuidance returns every prompt the user may give guidance to,
// with what they gave it.
func (s *SettingsService) ListPromptGuidance(ctx context.Context) ([]domain.PromptGuidance, error) {
	settings, err := s.promptGuidanceSettings(ctx)
	if err != nil {
		return nil, err
	}
	definitions := domain.PromptDefinitions()
	guidance := make([]domain.PromptGuidance, 0, len(definitions))
	for _, definition := range definitions {
		guidance = append(guidance, domain.GuidanceOf(definition, settings))
	}
	return guidance, nil
}

// PromptGuidance returns one prompt's guidance. A key that is not one of
// the prompts is ports.ErrNotFound.
func (s *SettingsService) PromptGuidance(ctx context.Context, key domain.PromptKey) (domain.PromptGuidance, error) {
	definition, err := promptDefinition(key)
	if err != nil {
		return domain.PromptGuidance{}, err
	}
	settings, err := s.promptGuidanceSettings(ctx)
	if err != nil {
		return domain.PromptGuidance{}, err
	}
	return domain.GuidanceOf(definition, settings), nil
}

// SavePromptGuidance replaces a prompt's guidance with the user's, and
// records which default they were shown so a later change to it can be
// pointed out (ADR-0031 §7).
func (s *SettingsService) SavePromptGuidance(ctx context.Context, key domain.PromptKey, text string) (domain.PromptGuidance, error) {
	definition, err := promptDefinition(key)
	if err != nil {
		return domain.PromptGuidance{}, err
	}
	text, err = domain.NormalizeGuidance(text)
	if err != nil {
		return domain.PromptGuidance{}, err
	}
	settings, err := s.promptGuidanceSettings(ctx)
	if err != nil {
		return domain.PromptGuidance{}, err
	}
	settings[key] = domain.StoredGuidance{Text: text, DefaultDigest: domain.GuidanceDigest(definition.Default)}
	if err := s.savePromptGuidanceSettings(ctx, settings); err != nil {
		return domain.PromptGuidance{}, err
	}
	return domain.GuidanceOf(definition, settings), nil
}

// ResetPromptGuidance removes the user's guidance for a prompt, which then
// follows its default, including as later releases change it.
func (s *SettingsService) ResetPromptGuidance(ctx context.Context, key domain.PromptKey) (domain.PromptGuidance, error) {
	definition, err := promptDefinition(key)
	if err != nil {
		return domain.PromptGuidance{}, err
	}
	settings, err := s.promptGuidanceSettings(ctx)
	if err != nil {
		return domain.PromptGuidance{}, err
	}
	if _, ok := settings[key]; ok {
		delete(settings, key)
		if err := s.savePromptGuidanceSettings(ctx, settings); err != nil {
			return domain.PromptGuidance{}, err
		}
	}
	return domain.GuidanceOf(definition, settings), nil
}

func promptDefinition(key domain.PromptKey) (domain.PromptDefinition, error) {
	definition, ok := domain.PromptDefinitionFor(key)
	if !ok {
		return domain.PromptDefinition{}, fmt.Errorf("no prompt is called %q: %w", key, ports.ErrNotFound)
	}
	return definition, nil
}

func (s *SettingsService) promptGuidanceSettings(ctx context.Context) (domain.PromptGuidanceSettings, error) {
	settings := domain.PromptGuidanceSettings{}
	value, found, err := s.repo.Get(ctx, promptGuidanceKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt guidance: %w", err)
	}
	if !found {
		return settings, nil
	}
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, fmt.Errorf("failed to parse stored prompt guidance: %w", err)
	}
	if settings == nil { // a stored "null"
		settings = domain.PromptGuidanceSettings{}
	}
	return settings, nil
}

func (s *SettingsService) savePromptGuidanceSettings(ctx context.Context, settings domain.PromptGuidanceSettings) error {
	value, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to encode prompt guidance: %w", err)
	}
	if err := s.repo.Set(ctx, promptGuidanceKey, string(value)); err != nil {
		return fmt.Errorf("failed to save prompt guidance: %w", err)
	}
	return nil
}
