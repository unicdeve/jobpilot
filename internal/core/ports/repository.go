package ports

import (
	"context"
	"github.com/unicdev/job-agent/internal/core/domain"
)

// CandidateRepository persists and retrieves candidate profiles.
type CandidateRepository interface {
	Save(ctx context.Context, candidate *domain.Candidate) error
	GetByID(ctx context.Context, id string) (*domain.Candidate, error)
	Update(ctx context.Context, candidate *domain.Candidate) error
	Delete(ctx context.Context, id string) error
	// GetFirstCandidate returns the first candidate profile in the
	// database. Job Agent currently manages a single local candidate
	// profile per installation, so this is the standard way to fetch
	// "the" profile.
	GetFirstCandidate(ctx context.Context) (*domain.Candidate, error)
}

// ResumeRepository persists and retrieves candidate resumes.
type ResumeRepository interface {
	Save(ctx context.Context, resume *domain.Resume) error
	GetByID(ctx context.Context, id string) (*domain.Resume, error)
	GetByCandidateID(ctx context.Context, candidateID string) ([]*domain.Resume, error)
	Delete(ctx context.Context, id string) error
}

// JobRepository persists and retrieves discovered job postings.
type JobRepository interface {
	Save(ctx context.Context, job *domain.Job) error
	GetByID(ctx context.Context, id string) (*domain.Job, error)
	GetByCanonicalURL(ctx context.Context, url string) (*domain.Job, error)
	List(ctx context.Context, limit, offset int) ([]*domain.Job, error)
	Delete(ctx context.Context, id string) error
}

// MatchRepository persists and retrieves job-match analyses.
type MatchRepository interface {
	Save(ctx context.Context, match *domain.JobMatch) error
	GetByID(ctx context.Context, id string) (*domain.JobMatch, error)
	GetByJobID(ctx context.Context, jobID string) (*domain.JobMatch, error)
	ListByCandidate(ctx context.Context, candidateID string) ([]*domain.JobMatch, error)
	Delete(ctx context.Context, id string) error
}
