package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/domain/geo"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// ErrInvalidTimeZone is returned for a time zone that is not an IANA zone
// JobPilot knows, such as Africa/Lagos.
var ErrInvalidTimeZone = errors.New("not a time zone JobPilot knows; use its IANA name, such as Africa/Lagos")

// normalizeTimeZone returns a zone's name as GeoNames writes it, and
// refuses one it does not list.
func normalizeTimeZone(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	zone, ok := geo.TimeZoneByID(name)
	if !ok {
		return "", fmt.Errorf("%q is %w", name, ErrInvalidTimeZone)
	}
	return zone.ID, nil
}

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
	zone, err := normalizeTimeZone(profile.TimeZone)
	if err != nil {
		return err
	}
	profile.TimeZone = zone

	if err := s.repo.Update(ctx, profile); err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}
	return nil
}
