// Package ports defines the interfaces (the "ports" in hexagonal
// architecture) that core services depend on, letting secondary adapters
// (SQLite, Ollama, job-board connectors, ...) be swapped without touching
// business logic.
package ports

import (
	"context"
	"errors"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// ErrSourceUnreachable marks a source that gave no answer at all: no
// connection, no name resolution, or no response before its timeout. It is
// what being offline looks like from inside a connector.
//
// A connector wraps the error from its HTTP client's Do with it, and only
// that error: a source that answered with an error status or with a body it
// could not read was reached, and failed in a way the user's internet
// connection does not explain.
var ErrSourceUnreachable = errors.New("source unreachable")

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
	// CanSearch reports whether Search can be called at all. A source
	// without it is skipped entirely and contributes no jobs.
	CanSearch bool
	// FiltersServerSide reports whether Search honours the criteria itself.
	//
	// When false — true of every public board wired up so far, since neither
	// Arbeitnow nor Remotive acts on its own query parameters —
	// JobDiscoveryService filters the returned jobs against the criteria
	// before any of them are stored. Connectors therefore do not each carry
	// their own copy of the matching rule, and a new connector cannot import
	// its whole board by forgetting to filter.
	//
	// Set it true only for a source whose API really does apply the
	// criteria; its results are then taken as-is.
	FiltersServerSide bool
	CanGetJob         bool
	CanAuthenticate   bool
}
