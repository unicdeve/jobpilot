package ports

import (
	"context"
	"errors"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// LocalStorage is where JobPilot keeps its data on this machine.
type LocalStorage interface {
	// Prepare makes the data directory and brings the database schema up
	// to date, returning the directory. It is safe to call on every start.
	Prepare(ctx context.Context) (string, error)
}

// ErrNoCandidate is returned by CandidateRepository.GetFirstCandidate when
// no profile exists yet. That is the ordinary state of a new installation,
// not a failure, so callers can tell it apart from a database error.
var ErrNoCandidate = errors.New("no candidate profile found")

// CandidateRepository persists and retrieves candidate profiles.
type CandidateRepository interface {
	Save(ctx context.Context, candidate *domain.Candidate) error
	GetByID(ctx context.Context, id string) (*domain.Candidate, error)
	Update(ctx context.Context, candidate *domain.Candidate) error
	// GetFirstCandidate returns the first candidate profile in the
	// database. JobPilot currently manages a single local candidate
	// profile per installation, so this is the standard way to fetch
	// "the" profile.
	GetFirstCandidate(ctx context.Context) (*domain.Candidate, error)
}

// ResumeRepository persists and retrieves candidate resumes.
type ResumeRepository interface {
	Save(ctx context.Context, resume *domain.Resume) error
	GetByID(ctx context.Context, id string) (*domain.Resume, error)
	GetByCandidateID(ctx context.Context, candidateID string) ([]*domain.Resume, error)
}

// JobRepository persists and retrieves discovered job postings.
type JobRepository interface {
	Save(ctx context.Context, job *domain.Job) error
	GetByID(ctx context.Context, id string) (*domain.Job, error)
	GetByCanonicalURL(ctx context.Context, url string) (*domain.Job, error)
	List(ctx context.Context, filter domain.JobFilter, limit, offset int) ([]*domain.Job, error)
	Count(ctx context.Context, filter domain.JobFilter) (int, error)
	// SetNotInterested marks a job not interested as of at, or clears the
	// mark when at is nil.
	SetNotInterested(ctx context.Context, id string, at *time.Time) error
}

// SavedSearchRepository persists the user's named search criteria.
type SavedSearchRepository interface {
	Save(ctx context.Context, search *domain.SavedSearch) error
	GetByID(ctx context.Context, id string) (*domain.SavedSearch, error)
	GetByName(ctx context.Context, name string) (*domain.SavedSearch, error)
	List(ctx context.Context) ([]*domain.SavedSearch, error)
	Delete(ctx context.Context, id string) error
	// ClearDefault unsets the default flag on every row, so a caller can set
	// a new default without tripping the single-default index.
	ClearDefault(ctx context.Context) error
	// MarkRun records when the search last ran against the job sources.
	// It changes nothing else, not even updated_at: running a search is not
	// editing it.
	MarkRun(ctx context.Context, id string, at time.Time) error
}

// CredentialNameRepository records which credentials exist, and never
// their values (ADR-0019 §6). It is a directory over the keychain, which
// cannot be enumerated.
type CredentialNameRepository interface {
	// Record notes that a credential exists for ref, or refreshes it.
	Record(ctx context.Context, ref domain.CredentialRef) error
	// Forget removes a ref from the directory.
	Forget(ctx context.Context, ref domain.CredentialRef) error
	// List returns every recorded ref, by source then label.
	List(ctx context.Context) ([]domain.CredentialRef, error)
}

// MatchRepository persists and retrieves job-match analyses.
type MatchRepository interface {
	Save(ctx context.Context, match *domain.JobMatch) error
	GetByID(ctx context.Context, id string) (*domain.JobMatch, error)
	GetByJobID(ctx context.Context, jobID string) (*domain.JobMatch, error)
	ListByCandidate(ctx context.Context, candidateID string) ([]*domain.JobMatch, error)
}

// SettingsRepository persists arbitrary local key/value settings.
type SettingsRepository interface {
	Get(ctx context.Context, key string) (value string, found bool, err error)
	Set(ctx context.Context, key, value string) error
}

// ScoutRepository persists and retrieves Scouts — the standing
// instructions that run a saved search and create applications to review
// (ADR-0018).
type ScoutRepository interface {
	Save(ctx context.Context, scout *domain.Scout) error
	GetByID(ctx context.Context, id string) (*domain.Scout, error)
	Update(ctx context.Context, scout *domain.Scout) error
	List(ctx context.Context) ([]*domain.Scout, error)
	Delete(ctx context.Context, id string) error
}
