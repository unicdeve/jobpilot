package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// ErrEmptyCoverLetter is returned for a letter with no text.
var ErrEmptyCoverLetter = errors.New("a cover letter cannot be empty")

// CoverLetterService keeps the cover letters the candidate writes for an
// application, as versions (docs/02_features/06_cover_letters.md).
//
// It never writes a letter: JobPilot does not draft text that goes to an
// employer (ADR-0034 §1). What it adds is the draft checks, which advise on
// the letter the candidate wrote.
type CoverLetterService struct {
	coverLetterRepo ports.CoverLetterRepository
	appRepo         ports.ApplicationRepository
	candRepo        ports.CandidateRepository
	resumeRepo      ports.ResumeRepository
	about           *AboutYouService
}

// NewCoverLetterService constructs a CoverLetterService.
func NewCoverLetterService(
	coverLetterRepo ports.CoverLetterRepository,
	appRepo ports.ApplicationRepository,
	candRepo ports.CandidateRepository,
	resumeRepo ports.ResumeRepository,
	about *AboutYouService,
) *CoverLetterService {
	return &CoverLetterService{
		coverLetterRepo: coverLetterRepo,
		appRepo:         appRepo,
		candRepo:        candRepo,
		resumeRepo:      resumeRepo,
		about:           about,
	}
}

// Create saves a letter the candidate wrote as a new version of an
// application's cover letter, and makes it the current one. Earlier
// versions are kept (AC-005).
func (s *CoverLetterService) Create(ctx context.Context, applicationID, content string) (*domain.CoverLetter, error) {
	content, err := letterContent(content)
	if err != nil {
		return nil, err
	}
	application, err := s.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch application: %w", err)
	}

	coverLetter := &domain.CoverLetter{ApplicationID: applicationID, Content: content}
	if err := s.coverLetterRepo.Save(ctx, coverLetter); err != nil {
		return nil, fmt.Errorf("failed to save cover letter: %w", err)
	}

	application.CurrentCoverLetterID = coverLetter.ID
	if err := s.appRepo.Update(ctx, application); err != nil {
		return nil, fmt.Errorf("failed to update application's current cover letter: %w", err)
	}

	if err := s.check(ctx, application, coverLetter); err != nil {
		return nil, err
	}
	return coverLetter, nil
}

// Edit changes an existing version in place (AC-004).
func (s *CoverLetterService) Edit(ctx context.Context, coverLetterID, content string) (*domain.CoverLetter, error) {
	content, err := letterContent(content)
	if err != nil {
		return nil, err
	}
	coverLetter, err := s.coverLetterRepo.GetByID(ctx, coverLetterID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cover letter: %w", err)
	}

	coverLetter.Content = content
	if err := s.coverLetterRepo.Update(ctx, coverLetter); err != nil {
		return nil, fmt.Errorf("failed to update cover letter: %w", err)
	}

	application, err := s.appRepo.GetByID(ctx, coverLetter.ApplicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch application: %w", err)
	}
	if err := s.check(ctx, application, coverLetter); err != nil {
		return nil, err
	}
	return coverLetter, nil
}

// ListVersions returns every version of an application's cover letter,
// oldest first.
func (s *CoverLetterService) ListVersions(ctx context.Context, applicationID string) ([]*domain.CoverLetter, error) {
	versions, err := s.coverLetterRepo.ListByApplication(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list cover letter versions: %w", err)
	}
	if len(versions) == 0 {
		return versions, nil
	}

	application, err := s.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch application: %w", err)
	}
	if err := s.check(ctx, application, versions...); err != nil {
		return nil, err
	}
	return versions, nil
}

// letterContent is a letter as it is kept: line endings normalized and the
// ends trimmed, and never empty.
func letterContent(content string) (string, error) {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if content == "" {
		return "", ErrEmptyCoverLetter
	}
	return content, nil
}

// check sets each letter's flags from its text as it is now
// (docs/02_features/19_draft_checks.md).
func (s *CoverLetterService) check(ctx context.Context, application *domain.Application, letters ...*domain.CoverLetter) error {
	basis, err := draftBasis(ctx, application, s.candRepo, s.resumeRepo, s.about, true)
	if err != nil {
		return fmt.Errorf("failed to check the cover letter: %w", err)
	}
	for _, letter := range letters {
		letter.Flags = domain.CheckDraft(letter.Content, basis)
	}
	return nil
}
