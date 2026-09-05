package services

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/conductor-oss/markitdown"
	"github.com/unicdev/job-agent/internal/core/domain"
	"github.com/unicdev/job-agent/internal/core/ports"
)

// ResumeService manages a candidate's imported resumes.
type ResumeService struct {
	resumeRepo ports.ResumeRepository
}

// NewResumeService constructs a ResumeService backed by the given resume
// repository.
func NewResumeService(repo ports.ResumeRepository) *ResumeService {
	return &ResumeService{
		resumeRepo: repo,
	}
}

// ImportResume converts a document to markdown and stores it.
func (s *ResumeService) ImportResume(ctx context.Context, candidateID string, filePath string) (*domain.Resume, error) {
	md := markitdown.New()
	content, err := md.Convert(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to convert resume to markdown: %w", err)
	}

	resume := &domain.Resume{
		CandidateID:      candidateID,
		Name:             filepath.Base(filePath),
		OriginalFilePath: filePath,
		MarkdownContent:  content.Markdown,
	}

	if err := s.resumeRepo.Save(ctx, resume); err != nil {
		return nil, fmt.Errorf("failed to save resume: %w", err)
	}

	return resume, nil
}

// GetResumesByCandidate retrieves all resumes associated with a candidate.
func (s *ResumeService) GetResumesByCandidate(ctx context.Context, candidateID string) ([]*domain.Resume, error) {
	resumes, err := s.resumeRepo.GetByCandidateID(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to get resumes for candidate: %w", err)
	}
	return resumes, nil
}

// GetResume retrieves a resume by ID.
func (s *ResumeService) GetResume(ctx context.Context, id string) (*domain.Resume, error) {
	resume, err := s.resumeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get resume: %w", err)
	}
	return resume, nil
}

// DeleteResume removes a resume.
func (s *ResumeService) DeleteResume(ctx context.Context, id string) error {
	if err := s.resumeRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete resume: %w", err)
	}
	return nil
}
