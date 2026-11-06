package services

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// fakePersonalAnswerRepository is an in-memory ports.PersonalAnswerRepository.
type fakePersonalAnswerRepository struct {
	rows  map[string]*domain.PersonalAnswer
	order []string
	next  int
	clock time.Time
}

func newFakePersonalAnswerRepository() *fakePersonalAnswerRepository {
	return &fakePersonalAnswerRepository{rows: map[string]*domain.PersonalAnswer{}, clock: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)}
}

func (f *fakePersonalAnswerRepository) list(match func(*domain.PersonalAnswer) bool) []*domain.PersonalAnswer {
	out := make([]*domain.PersonalAnswer, 0)
	for _, id := range f.order {
		if row := f.rows[id]; match(row) {
			copied := *row
			out = append(out, &copied)
		}
	}
	return out
}

func (f *fakePersonalAnswerRepository) ListByCandidate(_ context.Context, candidateID string) ([]*domain.PersonalAnswer, error) {
	return f.list(func(row *domain.PersonalAnswer) bool { return row.CandidateID == candidateID }), nil
}

func (f *fakePersonalAnswerRepository) GetByID(_ context.Context, id string) (*domain.PersonalAnswer, error) {
	row, ok := f.rows[id]
	if !ok {
		return nil, fmt.Errorf("personal answer not found: %s", id)
	}
	copied := *row
	return &copied, nil
}

func (f *fakePersonalAnswerRepository) Save(_ context.Context, answer *domain.PersonalAnswer) error {
	f.clock = f.clock.Add(time.Minute)
	if answer.ID == "" {
		f.next++
		answer.ID = fmt.Sprintf("answer-%d", f.next)
		answer.CreatedAt = f.clock
		f.order = append(f.order, answer.ID)
	} else if _, ok := f.rows[answer.ID]; !ok {
		return fmt.Errorf("personal answer not found: %s", answer.ID)
	}
	answer.UpdatedAt = f.clock
	copied := *answer
	f.rows[answer.ID] = &copied
	return nil
}

func (f *fakePersonalAnswerRepository) Delete(_ context.Context, id string) error {
	delete(f.rows, id)
	f.order = slices.DeleteFunc(f.order, func(existing string) bool { return existing == id })
	return nil
}

type aboutYouFixture struct {
	svc          *AboutYouService
	answers      *fakePersonalAnswerRepository
	questions    *fakeApplicationQuestionRepository
	candidates   *fakeCandidateRepository
	applications *fakeApplicationRepository
	jobs         *fakeJobRepository
}

func newAboutYouFixture(t *testing.T) *aboutYouFixture {
	t.Helper()
	ctx := context.Background()
	f := &aboutYouFixture{
		answers:      newFakePersonalAnswerRepository(),
		questions:    newFakeApplicationQuestionRepository(),
		candidates:   newFakeCandidateRepository(),
		applications: newFakeApplicationRepository(),
		jobs:         newFakeJobRepository(),
	}
	require.NoError(t, f.candidates.Save(ctx, &domain.Candidate{ID: "cand-1", FirstName: "Ada", Summary: "Backend engineer"}))
	require.NoError(t, f.jobs.Save(ctx, &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/1", Title: "Platform Engineer",
		CompanyName: "Acme", DescriptionMarkdown: "We care about developer tooling."}))
	require.NoError(t, f.applications.Save(ctx, &domain.Application{ID: "app-1", JobID: "job-1", CandidateID: "cand-1"}))
	f.svc = NewAboutYouService(f.answers, f.candidates, f.applications, f.questions)
	return f
}

func TestAboutYouService_Answers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("every standing question is offered, answered or not, in order", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)

		about, err := f.svc.Get(ctx)

		require.NoError(t, err)
		require.Len(t, about.Questions, len(domain.StandingQuestions))
		for i, question := range about.Questions {
			assert.Equal(t, domain.StandingQuestions[i].Key, question.Key)
			assert.Empty(t, question.Answer)
		}
		assert.Empty(t, about.Kept)
	})

	t.Run("an answer is set, changed and cleared", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)

		about, err := f.svc.Save(ctx, AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{
			domain.AnswerValues:     "Honest reviews",
			domain.AnswerLookingFor: "A team that ships weekly.",
		}})
		require.NoError(t, err)
		assert.Equal(t, "Honest reviews", answerFor(about, domain.AnswerValues))

		about, err = f.svc.Save(ctx, AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{domain.AnswerValues: "Kind, honest reviews"}})
		require.NoError(t, err)
		assert.Equal(t, "Kind, honest reviews", answerFor(about, domain.AnswerValues))
		assert.Equal(t, "A team that ships weekly.", answerFor(about, domain.AnswerLookingFor), "a question left out of an update is unchanged")

		about, err = f.svc.Save(ctx, AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{domain.AnswerValues: "  "}})
		require.NoError(t, err)
		assert.Empty(t, answerFor(about, domain.AnswerValues))
		rows, err := f.answers.ListByCandidate(ctx, "cand-1")
		require.NoError(t, err)
		require.Len(t, rows, 1, "a cleared answer is a deleted row, not an empty one")
		assert.Equal(t, domain.AnswerLookingFor, rows[0].Key)
	})

	t.Run("an update naming a question that does not exist changes nothing", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)

		_, err := f.svc.Save(ctx, AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{
			domain.AnswerValues: "Honest reviews",
			"favourite_colour":  "blue",
		}})

		require.ErrorIs(t, err, ErrUnknownPersonalAnswer)
		rows, _ := f.answers.ListByCandidate(ctx, "cand-1")
		assert.Empty(t, rows)
	})

	t.Run("kept answers and the removed model-only keys cannot be set as standing answers", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)

		for _, key := range []domain.PersonalAnswerKey{domain.AnswerKept, "writing_sample", "writing_preferences", "never_mention"} {
			_, err := f.svc.Save(ctx, AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{key: "x"}})
			assert.ErrorIs(t, err, ErrUnknownPersonalAnswer, string(key))
		}
	})
}

func answerFor(about *AboutYou, key domain.PersonalAnswerKey) string {
	for _, question := range about.Questions {
		if question.Key == key {
			return question.Answer
		}
	}
	return ""
}

func TestAboutYouService_Keep(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	answered := func(f *aboutYouFixture, id, answer string) {
		require.NoError(t, f.questions.Save(ctx, &domain.ApplicationQuestion{ID: id, ApplicationID: "app-1",
			Label: "Why do you want to work at Acme?", Answer: answer}))
	}

	t.Run("an answer the candidate wrote is kept with its question", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)
		answered(f, "q-1", "Because of the tooling team.")

		kept, err := f.svc.Keep(ctx, "q-1")

		require.NoError(t, err)
		assert.Equal(t, domain.AnswerKept, kept.Key)
		assert.Equal(t, "cand-1", kept.CandidateID, "kept for the candidate, not the one application")
		assert.Equal(t, "Why do you want to work at Acme?", kept.Question)
		assert.Equal(t, "Because of the tooling team.", kept.Answer)
		about, err := f.svc.Get(ctx)
		require.NoError(t, err)
		require.Len(t, about.Kept, 1)
	})

	t.Run("keeping the same question again replaces what was kept", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)
		answered(f, "q-1", "Because of the tooling team.")
		_, err := f.svc.Keep(ctx, "q-1")
		require.NoError(t, err)
		answered(f, "q-2", "A better answer.")

		_, err = f.svc.Keep(ctx, "q-2")

		require.NoError(t, err)
		about, err := f.svc.Get(ctx)
		require.NoError(t, err)
		require.Len(t, about.Kept, 1)
		assert.Equal(t, "A better answer.", about.Kept[0].Answer)
	})

	t.Run("a question with no answer cannot be kept", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)
		answered(f, "empty", " ")

		_, err := f.svc.Keep(ctx, "empty")

		assert.ErrorIs(t, err, ErrNotKeepable)
	})

	t.Run("a kept answer can be forgotten, and nothing else through that door", func(t *testing.T) {
		t.Parallel()
		f := newAboutYouFixture(t)
		answered(f, "q-1", "Because of the tooling team.")
		kept, err := f.svc.Keep(ctx, "q-1")
		require.NoError(t, err)
		_, err = f.svc.Save(ctx, AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{domain.AnswerValues: "Honesty"}})
		require.NoError(t, err)
		rows, _ := f.answers.ListByCandidate(ctx, "cand-1")
		var valuesID string
		for _, row := range rows {
			if row.Key == domain.AnswerValues {
				valuesID = row.ID
			}
		}

		assert.ErrorIs(t, f.svc.DeleteKept(ctx, valuesID), ErrUnknownPersonalAnswer)
		require.NoError(t, f.svc.DeleteKept(ctx, kept.ID))
		about, err := f.svc.Get(ctx)
		require.NoError(t, err)
		assert.Empty(t, about.Kept)
		assert.Equal(t, "Honesty", answerFor(about, domain.AnswerValues))
	})
}

func TestAboutYouService_OwnWordsAndKeptAnswers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newAboutYouFixture(t)
	_, err := f.svc.Save(ctx, AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{
		domain.AnswerProudestWork: "Cut deploys from an hour to six minutes.",
	}})
	require.NoError(t, err)
	require.NoError(t, f.questions.Save(ctx, &domain.ApplicationQuestion{ID: "q-1", ApplicationID: "app-1",
		Label: "Why do you want to join our platform team?", Answer: "I like building for engineers."}))
	_, err = f.svc.Keep(ctx, "q-1")
	require.NoError(t, err)

	words, err := f.svc.OwnWords(ctx, "cand-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"Cut deploys from an hour to six minutes.", "I like building for engineers."}, words)

	kept, err := f.svc.KeptAnswers(ctx, "cand-1")
	require.NoError(t, err)
	require.Len(t, kept, 1)
	assert.Equal(t, "Why do you want to join our platform team?", kept[0].Question)

	var none *AboutYouService
	words, err = none.OwnWords(ctx, "cand-1")
	require.NoError(t, err)
	assert.Empty(t, words, "a service built without About you has nothing to add")
	kept, err = none.KeptAnswers(ctx, "cand-1")
	require.NoError(t, err)
	assert.Empty(t, kept)
}
