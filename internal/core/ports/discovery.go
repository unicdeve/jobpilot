// Package ports defines the interfaces (the "ports" in hexagonal
// architecture) that core services depend on, letting secondary adapters
// (SQLite, Ollama, job-board connectors, ...) be swapped without touching
// business logic.
package ports

import (
	"context"
	"errors"
	"fmt"

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

// PartialAnswer is what a source made of several parts returns, beside the
// jobs the others found, when some of its parts did not answer: a watched
// company whose board has gone, say (ADR-0024 §5).
type PartialAnswer struct {
	// Failing names the parts that did not answer.
	Failing []string
	// Err is nil when some part answered. Otherwise it is the source's own
	// failure, wrapping ErrSourceUnreachable when no part could be reached.
	Err error
}

func (p *PartialAnswer) Error() string {
	if p.Err != nil {
		return p.Err.Error()
	}
	return fmt.Sprintf("%d parts did not answer", len(p.Failing))
}

func (p *PartialAnswer) Unwrap() error { return p.Err }

// ErrBoardGone is returned for a hiring page its provider says does not
// exist: the company moved provider, or closed its page.
var ErrBoardGone = errors.New("no hiring page at this address")

// ErrBoardBusy is returned for a hiring page whose provider asked JobPilot
// to wait before asking again: Workable's 429. The page is there, and is
// asked again at the next read (ADR-0026 §5).
var ErrBoardBusy = errors.New("the hiring page asked JobPilot to wait before asking again")

// BoardReader reads companies' hiring pages (ADR-0024 §4).
type BoardReader interface {
	// ReadBoard returns every open posting on the board. A board the
	// provider does not know is ErrBoardGone, and no answer at all wraps
	// ErrSourceUnreachable.
	ReadBoard(ctx context.Context, board domain.Board) (*domain.BoardContents, error)
	// DescribeJobs returns jobs, read from the board with ReadBoard, with
	// their descriptions. It is for a board whose contents are Undescribed
	// (ADR-0026 §2), and is asked only for the postings a search may keep.
	// A posting the provider no longer has is left out.
	DescribeJobs(ctx context.Context, board domain.Board, jobs []domain.Job) ([]domain.Job, error)
	// BoardName returns the company's name as its board gives it, or ""
	// when it gives none.
	BoardName(ctx context.Context, board domain.Board) (string, error)
	// ResolveAddress returns the address a provider's short address leads
	// to, which names the board: a Workable posting's apply.workable.com/j/…
	// (ADR-0026 §3). It follows one redirect and reads nothing else.
	ResolveAddress(ctx context.Context, address string) (string, error)
}
