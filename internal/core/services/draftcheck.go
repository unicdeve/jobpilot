package services

import (
	"context"
	"fmt"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// draftBasis is what an application's letter and written answers are
// checked against (docs/02_features/19_draft_checks.md): the candidate's
// facts, the resume the application uses, and what they said about
// themselves. A letter is also held to a length, which an answer is not.
//
// It is built on every read rather than stored (D3), so a draft is always
// checked against the profile as it is now.
func draftBasis(
	ctx context.Context,
	application *domain.Application,
	candidates ports.CandidateRepository,
	resumes ports.ResumeRepository,
	about *AboutYouService,
	forLetter bool,
) (domain.DraftBasis, error) {
	candidate, err := candidates.GetByID(ctx, application.CandidateID)
	if err != nil {
		return domain.DraftBasis{}, fmt.Errorf("failed to fetch candidate: %w", err)
	}
	material, err := about.Material(ctx, candidate.ID, application.ID)
	if err != nil {
		return domain.DraftBasis{}, err
	}

	ownWords := material.OwnWords()
	if application.ResumeID != "" {
		resume, err := resumes.GetByID(ctx, application.ResumeID)
		if err != nil {
			return domain.DraftBasis{}, fmt.Errorf("failed to fetch resume: %w", err)
		}
		ownWords = append(ownWords, resume.MarkdownContent)
	}

	basis := domain.DraftBasisFor(*candidate, time.Now(), ownWords...)
	if forLetter {
		basis.WordLimit, basis.LimitIsTheirs = domain.LetterWordLimit(material.WritingPreferences())
	}
	return basis, nil
}
