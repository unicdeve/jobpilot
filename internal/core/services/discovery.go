package services

import (
	"context"
	"fmt"

	"github.com/unicdev/job-agent/internal/core/domain"
	"github.com/unicdev/job-agent/internal/core/ports"
)

// JobDiscoveryService coordinates job search across multiple sources.
type JobDiscoveryService struct {
	repo    ports.JobRepository
	sources []ports.JobSource
}

func NewJobDiscoveryService(repo ports.JobRepository, sources []ports.JobSource) *JobDiscoveryService {
	return &JobDiscoveryService{
		repo:    repo,
		sources: sources,
	}
}

// Search executes a search across all registered sources and deduplicates results.
func (s *JobDiscoveryService) Search(ctx context.Context, criteria domain.SearchCriteria) ([]*domain.Job, error) {
	var allJobs []domain.Job

	// In a production app, these would run in parallel.
	for _, source := range s.sources {
		if !source.Capabilities().CanSearch {
			continue
		}

		jobs, err := source.Search(ctx, criteria)
		if err != nil {
			// Log error but continue with other sources
			fmt.Printf("Source %s failed search: %v\n", source.ID(), err)
			continue
		}
		allJobs = append(allJobs, jobs...)
	}

	var finalJobs []*domain.Job
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

// ImportJob imports a specific job by its URL.
func (s *JobDiscoveryService) ImportJob(ctx context.Context, url string) (*domain.Job, error) {
	// Find a source that can handle this URL
	var source ports.JobSource
	for _, src := range s.sources {
		if src.Capabilities().CanGetJob {
			source = src
			break
		}
	}

	if source == nil {
		return nil, fmt.Errorf("no available source supports importing jobs by URL")
	}

	job, err := source.GetJob(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to import job from source %s: %w", source.ID(), err)
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
