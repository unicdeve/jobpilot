package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// ErrNotKeepable is returned for a question whose answer cannot be kept: one
// with no answer yet.
var ErrNotKeepable = errors.New("only a question you have answered can be kept")

// ErrUnknownPersonalAnswer is returned for a key no question has, or an
// answer that is not this candidate's.
var ErrUnknownPersonalAnswer = errors.New("unknown personal answer")

// AboutYouService keeps what the candidate says about themselves
// (docs/02_features/16_about_you.md, ADR-0022): their answers to the About
// you questions, and the answers they wrote on an application and kept.
//
// Everything it stores is the candidate's own words, and nothing is
// generated from it (ADR-0034 §5). It is the candidate's copy-ready
// material, the source of kept-answer suggestions, and part of what the
// draft checks hold a claim against.
type AboutYouService struct {
	answers      ports.PersonalAnswerRepository
	candidates   ports.CandidateRepository
	applications ports.ApplicationRepository
	questions    ports.ApplicationQuestionRepository
}

// NewAboutYouService constructs an AboutYouService.
func NewAboutYouService(
	answers ports.PersonalAnswerRepository,
	candidates ports.CandidateRepository,
	applications ports.ApplicationRepository,
	questions ports.ApplicationQuestionRepository,
) *AboutYouService {
	return &AboutYouService{
		answers: answers, candidates: candidates, applications: applications, questions: questions,
	}
}

// AboutYou is everything the candidate has said about themselves: each
// standing question with its answer, answered or not, and the answers they
// kept.
type AboutYou struct {
	Questions []AboutYouAnswer         `json:"questions"`
	Kept      []*domain.PersonalAnswer `json:"kept"`
}

// AboutYouAnswer is a standing question and the candidate's answer to it.
type AboutYouAnswer struct {
	domain.StandingQuestion
	Answer string `json:"answer"`
}

// AboutYouUpdate changes some of what the candidate has said. A question
// left out is unchanged; an empty answer clears it.
type AboutYouUpdate struct {
	Answers map[domain.PersonalAnswerKey]string `json:"answers"`
}

// Get returns what the candidate has said about themselves.
func (s *AboutYouService) Get(ctx context.Context) (*AboutYou, error) {
	candidate, err := s.candidates.GetFirstCandidate(ctx)
	if err != nil {
		return nil, err
	}
	return s.forCandidate(ctx, candidate.ID)
}

func (s *AboutYouService) forCandidate(ctx context.Context, candidateID string) (*AboutYou, error) {
	rows, err := s.answers.ListByCandidate(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("failed to read what the candidate said about themselves: %w", err)
	}
	return aboutYouFrom(rows), nil
}

func aboutYouFrom(rows []*domain.PersonalAnswer) *AboutYou {
	byKey := map[domain.PersonalAnswerKey]string{}
	about := &AboutYou{Kept: make([]*domain.PersonalAnswer, 0)}
	for _, row := range rows {
		if row.Key == domain.AnswerKept {
			about.Kept = append(about.Kept, row)
			continue
		}
		byKey[row.Key] = row.Answer
	}
	for _, question := range domain.StandingQuestions {
		about.Questions = append(about.Questions, AboutYouAnswer{StandingQuestion: question, Answer: byKey[question.Key]})
	}
	return about
}

// Save applies an update and returns what the candidate has said now.
// Every key is checked before anything is written, so an update naming a
// question that does not exist changes nothing.
func (s *AboutYouService) Save(ctx context.Context, update AboutYouUpdate) (*AboutYou, error) {
	for key := range update.Answers {
		if !domain.IsStandingKey(key) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownPersonalAnswer, key)
		}
	}

	candidate, err := s.candidates.GetFirstCandidate(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.answers.ListByCandidate(ctx, candidate.ID)
	if err != nil {
		return nil, err
	}
	existing := map[domain.PersonalAnswerKey]*domain.PersonalAnswer{}
	for _, row := range rows {
		if row.Key != domain.AnswerKept {
			existing[row.Key] = row
		}
	}

	for _, question := range domain.StandingQuestions {
		answer, ok := update.Answers[question.Key]
		if !ok {
			continue
		}
		row := existing[question.Key]
		if strings.TrimSpace(answer) == "" {
			// An empty answer is not an answer (ADR-0022 §2).
			if row != nil {
				if err := s.answers.Delete(ctx, row.ID); err != nil {
					return nil, err
				}
			}
			continue
		}
		if row == nil {
			row = &domain.PersonalAnswer{CandidateID: candidate.ID, Key: question.Key}
		}
		row.Question, row.Answer = question.Question, answer
		if err := s.answers.Save(ctx, row); err != nil {
			return nil, err
		}
	}
	return s.forCandidate(ctx, candidate.ID)
}

// Keep stores the answer the candidate wrote to a question on an
// application, with the question, so a similar question later is offered
// it (docs/02_features/16_about_you.md D6). Keeping the same question again
// replaces what was kept for it.
func (s *AboutYouService) Keep(ctx context.Context, questionID string) (*domain.PersonalAnswer, error) {
	question, err := s.questions.GetByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(question.Answer) == "" {
		return nil, ErrNotKeepable
	}
	application, err := s.applications.GetByID(ctx, question.ApplicationID)
	if err != nil {
		return nil, err
	}

	rows, err := s.answers.ListByCandidate(ctx, application.CandidateID)
	if err != nil {
		return nil, err
	}
	kept := &domain.PersonalAnswer{CandidateID: application.CandidateID, Key: domain.AnswerKept}
	for _, row := range rows {
		if row.Key == domain.AnswerKept && strings.EqualFold(strings.TrimSpace(row.Question), strings.TrimSpace(question.Label)) {
			kept = row
			break
		}
	}
	kept.Question, kept.Answer = strings.TrimSpace(question.Label), question.Answer
	if err := s.answers.Save(ctx, kept); err != nil {
		return nil, err
	}
	return kept, nil
}

// DeleteKept forgets a kept answer.
func (s *AboutYouService) DeleteKept(ctx context.Context, id string) error {
	candidate, err := s.candidates.GetFirstCandidate(ctx)
	if err != nil {
		return err
	}
	row, err := s.answers.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row.Key != domain.AnswerKept || row.CandidateID != candidate.ID {
		return ErrUnknownPersonalAnswer
	}
	return s.answers.Delete(ctx, id)
}

// KeptAnswers returns the answers a candidate kept. A nil service has none.
func (s *AboutYouService) KeptAnswers(ctx context.Context, candidateID string) ([]domain.PersonalAnswer, error) {
	if s == nil {
		return nil, nil
	}
	about, err := s.forCandidate(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	kept := make([]domain.PersonalAnswer, 0, len(about.Kept))
	for _, answer := range about.Kept {
		kept = append(kept, *answer)
	}
	return kept, nil
}

// OwnWords is what the candidate said about themselves that a draft's
// claims may rest on (docs/02_features/19_draft_checks.md D1): their About
// you answers and the answers they kept. A nil service has none.
func (s *AboutYouService) OwnWords(ctx context.Context, candidateID string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	about, err := s.forCandidate(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	var words []string
	for _, question := range about.Questions {
		if question.Answer != "" {
			words = append(words, question.Answer)
		}
	}
	for _, answer := range about.Kept {
		words = append(words, answer.Answer)
	}
	return words, nil
}
