package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// allowedTransitions is the application state machine
// (docs/02_features/08_applications.md): an explicit, deterministic rule
// table, never delegated to the LLM (AGENTS.md: "deterministic business
// rules must not be delegated to an LLM"). SUBMITTED is reachable only from
// MATCHED and APPROVED, and only through TransitionState below, so an
// application can never be marked submitted as a side effect of anything
// else (ADR-0034 §3).
var allowedTransitions = map[domain.ApplicationState][]domain.ApplicationState{
	domain.ApplicationStateDiscovered: {domain.ApplicationStateMatched, domain.ApplicationStateWithdrawn},
	// APPROVED is "ready to send", which a user may skip by recording that
	// they sent it.
	domain.ApplicationStateMatched:  {domain.ApplicationStateApproved, domain.ApplicationStateSubmitted, domain.ApplicationStateWithdrawn},
	domain.ApplicationStateApproved: {domain.ApplicationStateSubmitted, domain.ApplicationStateMatched, domain.ApplicationStateWithdrawn},
	// Everything below SUBMITTED is an outcome the user reports, never one
	// the application works out for itself (08_applications.md AC-007).
	domain.ApplicationStateSubmitted:    {domain.ApplicationStateInterviewing, domain.ApplicationStateOffered, domain.ApplicationStateRejected, domain.ApplicationStateNoResponse, domain.ApplicationStateWithdrawn},
	domain.ApplicationStateInterviewing: {domain.ApplicationStateOffered, domain.ApplicationStateRejected, domain.ApplicationStateNoResponse, domain.ApplicationStateWithdrawn},
	domain.ApplicationStateOffered:      {domain.ApplicationStateRejected, domain.ApplicationStateWithdrawn},
	// A late reply reopens an application the user had given up on.
	domain.ApplicationStateNoResponse: {domain.ApplicationStateInterviewing, domain.ApplicationStateRejected},
	domain.ApplicationStateRejected:   {},
	domain.ApplicationStateWithdrawn:  {},
}

// AllowedTransitions returns the states an application in `from` may move
// to next, in the order they should be offered.
//
// It exists so a client can show only the actions that will work instead of
// every state there is, without keeping a second copy of the rule table
// that could disagree with this one. The result is derived from the
// application's state rather than from the state value alone so that
// per-application rules have somewhere to live later without changing the
// shape callers depend on.
func AllowedTransitions(application *domain.Application) []domain.ApplicationState {
	allowed := allowedTransitions[application.State]
	return slices.Clone(allowed)
}

// ApplicationService manages an application's lifecycle: creation and
// validated state transitions.
type ApplicationService struct {
	repo       ports.ApplicationRepository
	jobs       ports.JobRepository
	candidates ports.CandidateRepository
	resumes    ports.ResumeRepository
}

// NewApplicationService constructs an ApplicationService backed by the
// given application repository. The job, candidate and resume repositories
// are read to check that a new application names records that exist.
func NewApplicationService(repo ports.ApplicationRepository, jobs ports.JobRepository, candidates ports.CandidateRepository, resumes ports.ResumeRepository) *ApplicationService {
	return &ApplicationService{repo: repo, jobs: jobs, candidates: candidates, resumes: resumes}
}

// CreateApplication starts tracking a candidate's application to a job, in
// the initial DISCOVERED state. resumeID may be empty if no resume has been
// chosen yet.
//
// A job, candidate or resume that does not exist is a ports.ReferenceError,
// found before anything is saved rather than left to the database's foreign
// key, whose failure says nothing about which of the three was missing.
func (s *ApplicationService) CreateApplication(ctx context.Context, jobID, candidateID, resumeID string) (*domain.Application, error) {
	if _, err := s.jobs.GetByID(ctx, jobID); err != nil {
		return nil, fmt.Errorf("failed to create application: %w", ports.AsReference(err))
	}
	if _, err := s.candidates.GetByID(ctx, candidateID); err != nil {
		return nil, fmt.Errorf("failed to create application: %w", ports.AsReference(err))
	}
	if resumeID != "" {
		if _, err := s.resumes.GetByID(ctx, resumeID); err != nil {
			return nil, fmt.Errorf("failed to create application: %w", ports.AsReference(err))
		}
	}

	application := &domain.Application{
		JobID:       jobID,
		CandidateID: candidateID,
		ResumeID:    resumeID,
		State:       domain.ApplicationStateDiscovered,
	}

	if err := s.repo.Save(ctx, application); err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	return application, nil
}

// GetApplication retrieves an application by ID.
func (s *ApplicationService) GetApplication(ctx context.Context, id string) (*domain.Application, error) {
	application, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	return application, nil
}

// ErrInvalidApplyURL is returned for an apply link that is not an absolute
// http or https address.
var ErrInvalidApplyURL = errors.New("the application link must be a full http or https address")

// SetApplyURL records where the user says an application's form is, or
// clears it with an empty link, so "Open application page" opens the job's
// own link again (ADR-0034 §2).
//
// The link is opened in the user's browser, so it must be a web address:
// nothing the operating system would hand to another program.
func (s *ApplicationService) SetApplyURL(ctx context.Context, id, rawURL string) (*domain.Application, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL != "" && !isWebAddress(rawURL) {
		return nil, ErrInvalidApplyURL
	}

	application, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}
	application.ApplyURL = rawURL
	if err := s.repo.Update(ctx, application); err != nil {
		return nil, fmt.Errorf("failed to save the application link: %w", err)
	}
	return application, nil
}

func isWebAddress(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// ListApplications retrieves all applications for a candidate.
func (s *ApplicationService) ListApplications(ctx context.Context, candidateID string) ([]*domain.Application, error) {
	applications, err := s.repo.ListByCandidate(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}
	return applications, nil
}

// ListHistory retrieves the durable state-transition history for an
// application (docs/02_features/08_applications.md AC-003).
func (s *ApplicationService) ListHistory(ctx context.Context, applicationID string) ([]*domain.ApplicationStateTransition, error) {
	history, err := s.repo.ListStateTransitions(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list application history: %w", err)
	}
	return history, nil
}

// TransitionState moves an application to a new state, after validating
// that the transition is allowed from its current state (AC-001/AC-002).
func (s *ApplicationService) TransitionState(ctx context.Context, applicationID string, to domain.ApplicationState, reason string) (*domain.Application, error) {
	application, err := s.repo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}

	if !isTransitionAllowed(application.State, to) {
		return nil, fmt.Errorf("cannot transition application from %s to %s", application.State, to)
	}

	if err := s.repo.TransitionState(ctx, applicationID, application.State, to, reason); err != nil {
		return nil, fmt.Errorf("failed to transition application state: %w", err)
	}

	application.State = to
	return application, nil
}

func isTransitionAllowed(from, to domain.ApplicationState) bool {
	return slices.Contains(allowedTransitions[from], to)
}
