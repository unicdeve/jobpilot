package ports

import (
	"context"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// ApplicationRepository persists and retrieves applications and their
// state history.
type ApplicationRepository interface {
	Save(ctx context.Context, application *domain.Application) error
	GetByID(ctx context.Context, id string) (*domain.Application, error)
	// Update persists mutable Application fields other than State (which
	// only TransitionState may change).
	Update(ctx context.Context, application *domain.Application) error
	ListByCandidate(ctx context.Context, candidateID string) ([]*domain.Application, error)

	// TransitionState atomically moves an application from "from" to "to"
	// and appends an ApplicationStateTransition record, in a single
	// transaction. The update is conditional on the row's current state
	// still being "from": if it has changed since the caller read it, no
	// rows are affected and an error is returned, rather than silently
	// applying a transition the caller no longer has an accurate basis for.
	TransitionState(ctx context.Context, applicationID string, from, to domain.ApplicationState, reason string) error
	ListStateTransitions(ctx context.Context, applicationID string) ([]*domain.ApplicationStateTransition, error)
}

// CoverLetterRepository persists and retrieves an application's cover
// letter versions.
type CoverLetterRepository interface {
	// Save inserts a new cover letter version.
	Save(ctx context.Context, coverLetter *domain.CoverLetter) error
	GetByID(ctx context.Context, id string) (*domain.CoverLetter, error)
	// Update edits an existing version in place, rather than creating a
	// new version.
	Update(ctx context.Context, coverLetter *domain.CoverLetter) error
	ListByApplication(ctx context.Context, applicationID string) ([]*domain.CoverLetter, error)
}

// ApplicationQuestionRepository persists and retrieves the questions the
// candidate notes on an application, with their answers.
type ApplicationQuestionRepository interface {
	Save(ctx context.Context, question *domain.ApplicationQuestion) error
	GetByID(ctx context.Context, id string) (*domain.ApplicationQuestion, error)
	Update(ctx context.Context, question *domain.ApplicationQuestion) error
	// ListByApplication returns an application's questions, oldest first.
	ListByApplication(ctx context.Context, applicationID string) ([]*domain.ApplicationQuestion, error)
	Delete(ctx context.Context, id string) error
}
