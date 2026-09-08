// Package services implements the application's use cases (business logic)
// on top of the core/ports interfaces, independent of any CLI, HTTP, or
// storage concern.
package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// JobDiscoveryService coordinates job search across multiple sources.
type JobDiscoveryService struct {
	repo    ports.JobRepository
	sources []ports.JobSource
}

// NewJobDiscoveryService constructs a JobDiscoveryService backed by the given
// job repository and search sources.
func NewJobDiscoveryService(repo ports.JobRepository, sources []ports.JobSource) *JobDiscoveryService {
	return &JobDiscoveryService{
		repo:    repo,
		sources: sources,
	}
}

// Search executes a search across all registered sources and deduplicates results.
func (s *JobDiscoveryService) Search(ctx context.Context, criteria domain.SearchCriteria) ([]*domain.Job, error) {
	allJobs := make([]domain.Job, 0)

	// In a production app, these would run in parallel.
	for _, source := range s.sources {
		if !source.Capabilities().CanSearch {
			continue
		}

		jobs, err := source.Search(ctx, criteria)
		if err != nil {
			// Log and continue with other sources: a single source failing
			// shouldn't fail the whole search.
			slog.WarnContext(ctx, "job source search failed", "source", source.ID(), "error", err)
			continue
		}
		allJobs = append(allJobs, jobs...)
	}

	finalJobs := make([]*domain.Job, 0, len(allJobs))
	for _, job := range allJobs {
		// Deduplicate by canonical URL
		existing, err := s.repo.GetByCanonicalURL(ctx, job.CanonicalURL)
		if err != nil {
			return nil, fmt.Errorf("failed to check for duplicate job: %w", err)
		}

		if existing != nil {
			finalJobs = append(finalJobs, existing)
			continue
		}

		// New job, save it
		jobCopy := job
		if err := s.repo.Save(ctx, &jobCopy); err != nil {
			return nil, fmt.Errorf("failed to save discovered job: %w", err)
		}
		finalJobs = append(finalJobs, &jobCopy)
	}

	return finalJobs, nil
}

// ImportJob imports a specific job by its URL, trying every source capable
// of URL lookup in turn: one source not recognizing this URL shouldn't stop
// another from resolving it (the same failure-isolation principle Search
// already applies across sources).
func (s *JobDiscoveryService) ImportJob(ctx context.Context, url string) (*domain.Job, error) {
	var job *domain.Job
	var lastErr error
	attempted := false

	for _, src := range s.sources {
		if !src.Capabilities().CanGetJob {
			continue
		}
		attempted = true

		var err error
		job, err = src.GetJob(ctx, url)
		if err != nil {
			lastErr = fmt.Errorf("failed to import job from source %s: %w", src.ID(), err)
			continue
		}
		lastErr = nil
		break
	}

	if !attempted {
		return nil, fmt.Errorf("no available source supports importing jobs by URL")
	}
	if lastErr != nil {
		return nil, lastErr
	}

	// Deduplicate and save
	existing, err := s.repo.GetByCanonicalURL(ctx, job.CanonicalURL)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicate job: %w", err)
	}

	if existing != nil {
		return existing, nil
	}

	if err := s.repo.Save(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to save imported job: %w", err)
	}

	return job, nil
}
