package services

import (
	"context"
	"fmt"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// ProfileService manages the candidate's profile.
type ProfileService struct {
	repo ports.CandidateRepository
}

// NewProfileService constructs a ProfileService backed by the given
// candidate repository.
func NewProfileService(repo ports.CandidateRepository) *ProfileService {
	return &ProfileService{
		repo: repo,
	}
}

// GetProfile retrieves the candidate profile by ID.
func (s *ProfileService) GetProfile(ctx context.Context, id string) (*domain.Candidate, error) {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}
	return profile, nil
}

// UpdateProfile updates an existing candidate profile.
func (s *ProfileService) UpdateProfile(ctx context.Context, profile *domain.Candidate) error {
	if profile.ID == "" {
		return fmt.Errorf("profile ID is required for update")
	}

	if err := s.repo.Update(ctx, profile); err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}
	return nil
}

// DeleteProfile removes a candidate profile.
func (s *ProfileService) DeleteProfile(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}
	return nil
}
