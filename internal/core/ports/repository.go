package ports

import (
	"context"
	"github.com/unicdev/job-agent/internal/core/domain"
)

type CandidateRepository interface {
	Save(ctx context.Context, candidate *domain.Candidate) error
	GetByID(ctx context.Context, id string) (*domain.Candidate, error)
	Update(ctx context.Context, candidate *domain.Candidate) error
	Delete(ctx context.Context, id string) error
}

type ResumeRepository interface {
	Save(ctx context.Context, resume *domain.Resume) error
	GetByID(ctx context.Context, id string) (*domain.Resume, error)
	Delete(ctx context.Context, id string) error
}
