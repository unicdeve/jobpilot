package services

import (
	"context"
	"fmt"

	"github.com/unicdev/jobpilot/internal/adapters/secondary/sqlite"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// OnboardingService walks a new user through first-run setup: local storage,
// LLM connectivity, and the initial candidate profile.
type OnboardingService struct {
	candidateRepo ports.CandidateRepository
	resumeSvc     *ResumeService
	llm           ports.LLMProvider
	sqliteDB      *sqlite.DB
}

// NewOnboardingService constructs an OnboardingService with its required
// dependencies.
func NewOnboardingService(candidateRepo ports.CandidateRepository, resumeSvc *ResumeService, llm ports.LLMProvider, db *sqlite.DB) *OnboardingService {
	return &OnboardingService{
		candidateRepo: candidateRepo,
		resumeSvc:     resumeSvc,
		llm:           llm,
		sqliteDB:      db,
	}
}

// SetupEnvironment initializes the local storage and database.
func (s *OnboardingService) SetupEnvironment(ctx context.Context) (string, error) {
	dir, err := system.EnsureDataDir()
	if err != nil {
		return "", fmt.Errorf("failed to ensure data directory: %w", err)
	}

	// In a real app, the DB connection would be managed by a lifecycle manager.
	// For onboarding, we ensure the schema is initialized.
	if err := s.sqliteDB.InitializeSchema(ctx); err != nil {
		return "", fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return dir, nil
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

// ProposeProfileDraft asks the LLM to suggest a candidate Summary and Skills
// list from resume content, for the user to review before it is applied.
func (s *OnboardingService) ProposeProfileDraft(ctx context.Context, resumeMarkdown string) (*domain.ProfileDraft, error) {
	return s.resumeSvc.ProposeProfileDraft(ctx, resumeMarkdown)
}
