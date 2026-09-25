package services

import (
	"context"
	"fmt"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// OnboardingService walks a new user through first-run setup: local storage,
// LLM connectivity, and the initial candidate profile.
type OnboardingService struct {
	candidateRepo ports.CandidateRepository
	resumeSvc     *ResumeService
	savedSearches *SavedSearchService
	llm           ports.LLMProvider
	storage       ports.LocalStorage
}

// NewOnboardingService constructs an OnboardingService with its required
// dependencies.
func NewOnboardingService(candidateRepo ports.CandidateRepository, resumeSvc *ResumeService, savedSearches *SavedSearchService, llm ports.LLMProvider, storage ports.LocalStorage) *OnboardingService {
	return &OnboardingService{
		candidateRepo: candidateRepo,
		resumeSvc:     resumeSvc,
		savedSearches: savedSearches,
		llm:           llm,
		storage:       storage,
	}
}

// ConfigureSearchPreferences saves the first search the user wants to run,
// completing the "configure job-search preferences" step that
// docs/02_features/01_onboarding.md requires.
//
// It matters more than a convenience: discovery refuses to run without
// criteria, so an installation with no saved search cannot find any jobs at
// all. Setting one here is what makes the application usable on first run.
func (s *OnboardingService) ConfigureSearchPreferences(ctx context.Context, name string, criteria domain.SearchCriteria) (*domain.SavedSearch, error) {
	search, err := s.savedSearches.Create(ctx, name, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to save search preferences: %w", err)
	}
	return search, nil
}

// SetupEnvironment initializes the local storage and database.
func (s *OnboardingService) SetupEnvironment(ctx context.Context) (string, error) {
	return s.storage.Prepare(ctx)
}

// ConfigureLLM tests connectivity to the configured LLM provider.
func (s *OnboardingService) ConfigureLLM(ctx context.Context) error {
	if err := s.llm.TestConnectivity(ctx); err != nil {
		return fmt.Errorf("LLM connectivity test failed: %w", err)
	}

	return nil
}

// InitializeProfile creates the initial candidate profile. It mutates
// profile in place so callers observe the generated ID after save.
func (s *OnboardingService) InitializeProfile(ctx context.Context, profile *domain.Candidate) error {
	if err := s.candidateRepo.Save(ctx, profile); err != nil {
		return fmt.Errorf("failed to save initial profile: %w", err)
	}

	return nil
}

// ProcessResume converts a resume file to markdown and stores it.
func (s *OnboardingService) ProcessResume(ctx context.Context, candidateID string, filePath string) (*domain.Resume, error) {
	return s.resumeSvc.ImportResume(ctx, candidateID, filePath)
}

// ProposeProfileDraft delegates to ResumeService.ProposeProfileDraft.
func (s *OnboardingService) ProposeProfileDraft(ctx context.Context, resumeMarkdown string) (*domain.ProfileDraft, error) {
	return s.resumeSvc.ProposeProfileDraft(ctx, resumeMarkdown)
}
