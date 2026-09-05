// Package ports defines the interfaces (the "ports" in hexagonal
// architecture) that core services depend on, letting secondary adapters
// (SQLite, Ollama, job-board connectors, ...) be swapped without touching
// business logic.
package ports

import (
	"context"
	"github.com/unicdev/job-agent/internal/core/domain"
)

// JobSource defines the interface for a job discovery connector.
type JobSource interface {
	// ID returns a unique identifier for the source (e.g., "linkedin", "indeed").
	ID() string
	// Name returns a human-readable name for the source.
	Name() string
	// Capabilities returns the features supported by this source.
	Capabilities() SourceCapabilities
	// Search finds jobs matching the given criteria.
	Search(ctx context.Context, criteria domain.SearchCriteria) ([]domain.Job, error)
	// GetJob retrieves a specific job by its URL.
	GetJob(ctx context.Context, url string) (*domain.Job, error)
}

// SourceCapabilities defines what a JobSource can actually do.
type SourceCapabilities struct {
	CanSearch       bool
	CanGetJob       bool
	CanAuthenticate bool
}
