package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// SavedSearchService manages the named search criteria that drive discovery.
//
// Discovery is only ever run against criteria the user chose, so this is
// where "what am I looking for" is decided and kept. See
// docs/02_features/04_job_discovery.md AC-001 and the onboarding
// requirement in docs/02_features/01_onboarding.md.
type SavedSearchService struct {
	repo ports.SavedSearchRepository
}

// NewSavedSearchService constructs a SavedSearchService over repo.
func NewSavedSearchService(repo ports.SavedSearchRepository) *SavedSearchService {
	return &SavedSearchService{repo: repo}
}

// Create saves a new named search. The first search created becomes the
// default, so a user who sets up exactly one never has to also nominate it.
func (s *SavedSearchService) Create(ctx context.Context, name string, criteria domain.SearchCriteria) (*domain.SavedSearch, error) {
	name = strings.TrimSpace(name)
	if err := s.validate(ctx, name, criteria, ""); err != nil {
		return nil, err
	}

	existing, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing saved searches: %w", err)
	}

	search := &domain.SavedSearch{
		Name:      name,
		Criteria:  criteria,
		IsDefault: len(existing) == 0,
	}
	if err := s.repo.Save(ctx, search); err != nil {
		return nil, err
	}
	return search, nil
}

// Update replaces a saved search's name and criteria, leaving its default
// status alone — that is SetDefault's job.
func (s *SavedSearchService) Update(ctx context.Context, id, name string, criteria domain.SearchCriteria) (*domain.SavedSearch, error) {
	name = strings.TrimSpace(name)
	if err := s.validate(ctx, name, criteria, id); err != nil {
		return nil, err
	}

	search, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	search.Name = name
	search.Criteria = criteria
	if err := s.repo.Save(ctx, search); err != nil {
		return nil, err
	}
	return search, nil
}

// validate enforces the two rules that keep a saved search usable: it has a
// name to pick it by, and criteria that actually narrow anything. excludeID
// is the search being updated, whose own name should not count as taken.
func (s *SavedSearchService) validate(ctx context.Context, name string, criteria domain.SearchCriteria, excludeID string) error {
	if name == "" {
		return fmt.Errorf("a saved search needs a name")
	}
	if criteria.IsEmpty() {
		return fmt.Errorf("a saved search needs at least one criterion: searching with none would import every job on every board, not the jobs you want")
	}

	clash, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to check whether %q is already used: %w", name, err)
	}
	if clash != nil && clash.ID != excludeID {
		return fmt.Errorf("a saved search named %q already exists", name)
	}
	return nil
}

// List returns every saved search, default first.
func (s *SavedSearchService) List(ctx context.Context) ([]*domain.SavedSearch, error) {
	return s.repo.List(ctx)
}

// Get returns one saved search by ID.
func (s *SavedSearchService) Get(ctx context.Context, id string) (*domain.SavedSearch, error) {
	return s.repo.GetByID(ctx, id)
}

// Default returns the search discovery runs when the user does not name one,
// or nil when none is configured — the state in which discovery is refused.
func (s *SavedSearchService) Default(ctx context.Context) (*domain.SavedSearch, error) {
	searches, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, search := range searches {
		if search.IsDefault {
			return search, nil
		}
	}
	return nil, nil
}

// SetDefault promotes one saved search to be the default, demoting whichever
// held it before.
func (s *SavedSearchService) SetDefault(ctx context.Context, id string) (*domain.SavedSearch, error) {
	search, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ClearDefault(ctx); err != nil {
		return nil, err
	}

	search.IsDefault = true
	if err := s.repo.Save(ctx, search); err != nil {
		return nil, err
	}
	return search, nil
}

// Delete removes a saved search. Deleting the default promotes the next
// remaining search rather than leaving the user with searches but no
// default, which would make an unqualified discovery run fail for a reason
// they would have to go looking for.
func (s *SavedSearchService) Delete(ctx context.Context, id string) error {
	search, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if !search.IsDefault {
		return nil
	}

	remaining, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to pick a new default saved search: %w", err)
	}
	if len(remaining) == 0 {
		return nil
	}

	remaining[0].IsDefault = true
	return s.repo.Save(ctx, remaining[0])
}
