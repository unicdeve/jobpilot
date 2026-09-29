package services

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	now  func() time.Time
}

// NewSavedSearchService constructs a SavedSearchService over repo.
func NewSavedSearchService(repo ports.SavedSearchRepository) *SavedSearchService {
	return &SavedSearchService{repo: repo, now: time.Now}
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

// RecordRun records that a search ran against the job sources now. A caller
// records a run only when discovery answered: a run no source answered —
// offline, say — found nothing, and leaves the search as stale as it was.
func (s *SavedSearchService) RecordRun(ctx context.Context, id string) error {
	return s.repo.MarkRun(ctx, id, s.now())
}

// Stale returns the searches due to run again (SavedSearch.Stale), in the
// order List gives them: the default first.
func (s *SavedSearchService) Stale(ctx context.Context) ([]*domain.SavedSearch, error) {
	searches, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	stale := make([]*domain.SavedSearch, 0, len(searches))
	for _, search := range searches {
		if search.Stale(now) {
			stale = append(stale, search)
		}
	}
	return stale, nil
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

// Templates returns the catalog of ready-made searches a user can adopt.
//
// It is served from here, rather than written into each client, so that the
// CLI and the desktop application offer the same searches and a correction
// reaches both at once.
func (s *SavedSearchService) Templates() []domain.SearchTemplateGroup {
	return domain.SearchTemplateGroups()
}

// ImportTemplates creates one saved search per template, in the order given.
//
// The searches are ordinary ones from the moment they exist: the user edits,
// renames and deletes them like any other, and nothing records where they
// came from. A template is a starting point, not a subscription.
//
// Every ID is resolved before anything is written, so a mistyped one costs
// nothing. A repository failure partway through keeps the searches already
// created — the alternative, deleting them, discards work the user asked for
// to tidy up an error they can simply retry past.
func (s *SavedSearchService) ImportTemplates(ctx context.Context, templateIDs []string) ([]*domain.SavedSearch, error) {
	if len(templateIDs) == 0 {
		return nil, fmt.Errorf("no templates chosen")
	}

	templates := make([]domain.SearchTemplate, 0, len(templateIDs))
	seen := make(map[string]bool, len(templateIDs))
	for _, id := range templateIDs {
		if seen[id] {
			continue
		}
		seen[id] = true

		template, err := domain.FindSearchTemplate(id)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}

	existing, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing saved searches: %w", err)
	}
	taken := make(map[string]bool, len(existing))
	for _, search := range existing {
		taken[strings.ToLower(search.Name)] = true
	}

	created := make([]*domain.SavedSearch, 0, len(templates))
	for _, template := range templates {
		name := availableName(template.Name, taken)
		taken[strings.ToLower(name)] = true

		search, err := s.Create(ctx, name, template.Criteria)
		if err != nil {
			return created, fmt.Errorf("failed to import template %q: %w", template.ID, err)
		}
		created = append(created, search)
	}
	return created, nil
}

// availableName returns base, or base with the lowest numeric suffix that is
// free. Saved search names are unique, and importing a template twice is a
// reasonable thing to do — someone who has narrowed "Backend Engineer" to
// Berlin may well want the unnarrowed one back — so a second copy is
// numbered rather than refused.
func availableName(base string, taken map[string]bool) string {
	if !taken[strings.ToLower(base)] {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s %d", base, n)
		if !taken[strings.ToLower(candidate)] {
			return candidate
		}
	}
}
