package services

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/conductor-oss/markitdown"
	"github.com/unicdev/job-agent/internal/adapters/secondary/sqlite"
	"github.com/unicdev/job-agent/internal/adapters/secondary/system"
	"github.com/unicdev/job-agent/internal/core/domain"
	"github.com/unicdev/job-agent/internal/core/ports"
)

type OnboardingService struct {
	candidateRepo ports.CandidateRepository
	resumeRepo    ports.ResumeRepository
	llm           ports.LLMProvider
	sqliteDB      *sqlite.DB
}

func NewOnboardingService(candidateRepo ports.CandidateRepository, resumeRepo ports.ResumeRepository, llm ports.LLMProvider, db *sqlite.DB) *OnboardingService {
	return &OnboardingService{
		candidateRepo: candidateRepo,
		resumeRepo:    resumeRepo,
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
	if err := s.sqliteDB.InitializeSchema(); err != nil {
		return "", fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return dir, nil
}

// ConfigureLLM tests connectivity to the specified LLM provider.
func (s *OnboardingService) ConfigureLLM(ctx context.Context, provider string) error {
	// For MVP, we only support "ollama".
	if provider != "ollama" {
		return fmt.Errorf("unsupported LLM provider: %s", provider)
	}

	if err := s.llm.TestConnectivity(ctx); err != nil {
		return fmt.Errorf("LLM connectivity test failed: %w", err)
	}

	return nil
}

// InitializeProfile creates the initial candidate profile.
func (s *OnboardingService) InitializeProfile(ctx context.Context, profile domain.Candidate) error {
	if err := s.candidateRepo.Save(ctx, &profile); err != nil {
		return fmt.Errorf("failed to save initial profile: %w", err)
	}

	return nil
}

// ProcessResume converts a resume file to markdown and stores it.
func (s *OnboardingService) ProcessResume(ctx context.Context, candidateID string, filePath string) error {
	md := markitdown.New()
	content, err := md.Convert(filePath)
	if err != nil {
		return fmt.Errorf("failed to convert resume to markdown: %w", err)
	}

	resume := &domain.Resume{
		CandidateID:      candidateID,
		Name:             filepath.Base(filePath),
		OriginalFilePath: filePath,
		MarkdownContent:  content.Markdown,
	}

	if err := s.resumeRepo.Save(ctx, resume); err != nil {
		return fmt.Errorf("failed to save converted resume: %w", err)
	}

	return nil
}
