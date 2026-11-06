package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func newTestAnswerService(t *testing.T) (*AnswerService, *fakeApplicationQuestionRepository, *fakeApplicationRepository, *fakeJobRepository, *fakeCandidateRepository) {
	t.Helper()
	questionRepo := newFakeApplicationQuestionRepository()
	appRepo := newFakeApplicationRepository()
	jobRepo := newFakeJobRepository()
	candRepo := newFakeCandidateRepository()
	return NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, newFakeResumeRepository(), nil), questionRepo, appRepo, jobRepo, candRepo
}

// The label-by-label specification for profileFactAnswer now lives in
// answers_forms_test.go, written against the question labels of real
// recorded forms rather than invented ones.
//
// The test that stood here asserted the behaviour ADR-0015 removed: that a
// visa question is answered with a prose work-authorization field, that
// "Expected salary" is answered from the profile, and that any link
// question is answered from one links string. Those are the defects, not a
// contract to preserve.

func TestDerivedAnswer(t *testing.T) {
	t.Parallel()

	t.Run("remote-open candidate", func(t *testing.T) {
		t.Parallel()
		source, answer := derivedAnswer(&domain.Candidate{RemotePreference: domain.RemotePreferenceRemote}, "Are you open to remote work?")
		assert.Equal(t, "candidate.remote_preference", source)
		assert.Equal(t, "Yes, I am looking for remote work.", answer)
	})

	t.Run("an onsite candidate answers honestly rather than falling through", func(t *testing.T) {
		t.Parallel()
		// This case used to assert no answer, because the rule read a
		// prose field and could not tell "prefers onsite" from "has not
		// said". A structured preference can, and an honest no is worth as
		// much to the user as an honest yes.
		source, answer := derivedAnswer(&domain.Candidate{RemotePreference: domain.RemotePreferenceOnsite}, "Are you open to remote work?")
		assert.Equal(t, "candidate.remote_preference", source)
		assert.Equal(t, "I am looking for onsite work.", answer)
	})

	t.Run("a candidate who has not said falls through to the user", func(t *testing.T) {
		t.Parallel()
		source, answer := derivedAnswer(&domain.Candidate{}, "Are you open to remote work?")
		assert.Empty(t, source)
		assert.Empty(t, answer, "not having said is a state of its own, and is not a No")
	})

	t.Run("unrelated question falls through", func(t *testing.T) {
		t.Parallel()
		source, answer := derivedAnswer(&domain.Candidate{RemotePreference: domain.RemotePreferenceRemote}, "How many years of Go experience do you have?")
		assert.Empty(t, source)
		assert.Empty(t, answer)
	})
}

// setupAnswers is an answer service over one application, for a candidate
// whose profile answers some questions and not others.
func setupAnswers(t *testing.T) (*AnswerService, *fakeApplicationQuestionRepository) {
	t.Helper()
	ctx := context.Background()
	svc, questionRepo, appRepo, jobRepo, candRepo := newTestAnswerService(t)
	require.NoError(t, jobRepo.Save(ctx, &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1", Locations: []string{"Berlin, Germany"}}))
	require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1", Email: "ada@example.com",
		RemotePreference:   domain.RemotePreferenceRemote,
		WorkAuthorizations: []domain.WorkAuthorization{{Country: "Germany", IsAuthorized: true}}}))
	require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", JobID: "job-1", CandidateID: "cand-1"}))
	return svc, questionRepo
}

func TestAnswerService_AddQuestion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("a question is noted unanswered, oldest first", func(t *testing.T) {
		t.Parallel()
		svc, _ := setupAnswers(t)

		first, err := svc.AddQuestion(ctx, "app-1", "  Why do you want to work here?  ")
		require.NoError(t, err)
		_, err = svc.AddQuestion(ctx, "app-1", "Email address")
		require.NoError(t, err)

		assert.Equal(t, "Why do you want to work here?", first.Label, "the label is trimmed")
		assert.Empty(t, first.Answer)
		questions, err := svc.ListByApplication(ctx, "app-1")
		require.NoError(t, err)
		require.Len(t, questions, 2)
		assert.Equal(t, "Why do you want to work here?", questions[0].Label)
		assert.Equal(t, "Email address", questions[1].Label)
	})

	t.Run("an empty question is refused", func(t *testing.T) {
		t.Parallel()
		svc, questionRepo := setupAnswers(t)

		_, err := svc.AddQuestion(ctx, "app-1", "   ")

		assert.ErrorIs(t, err, ErrEmptyQuestion)
		assert.Empty(t, questionRepo.questions)
	})

	t.Run("an application that does not exist is named", func(t *testing.T) {
		t.Parallel()
		svc, _ := setupAnswers(t)

		_, err := svc.AddQuestion(ctx, "missing-app", "Email")

		assert.ErrorIs(t, err, ports.ErrNotFound)
	})
}

// Suggestions come only from the candidate's own material, and JobPilot
// never fills an answer in (ADR-0034 §4).
func TestAnswerService_Suggestions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, tt := range []struct {
		label  string
		want   *domain.AnswerSuggestion
		reason string
	}{
		{
			label:  "Email address",
			want:   &domain.AnswerSuggestion{Answer: "ada@example.com", Source: domain.SuggestionFromProfile, Field: "candidate.email"},
			reason: "a fact the profile states",
		},
		{
			label: "Are you authorized to work in Germany?",
			want: &domain.AnswerSuggestion{Answer: "Yes", Source: domain.SuggestionFromProfile,
				Field: "candidate.work_authorization.germany"},
			reason: "work authorization for a country the job names and the candidate spoke about",
		},
		{
			label:  "Are you open to remote work?",
			want:   &domain.AnswerSuggestion{Answer: "Yes, I am looking for remote work.", Source: domain.SuggestionDerived, Field: "candidate.remote_preference"},
			reason: "worked out from the profile",
		},
		{label: "What is your driver's license number?", reason: "a fact the profile does not hold is never guessed"},
		{label: "Why do you want to work here?", reason: "nothing writes a narrative answer"},
	} {
		t.Run(tt.label, func(t *testing.T) {
			t.Parallel()
			svc, _ := setupAnswers(t)

			question, err := svc.AddQuestion(ctx, "app-1", tt.label)

			require.NoError(t, err)
			assert.Equal(t, tt.want, question.Suggestion, tt.reason)
			assert.Empty(t, question.Answer, "a suggestion is offered, never filled in")
		})
	}

	t.Run("an answer the candidate kept is suggested for a similar question", func(t *testing.T) {
		t.Parallel()
		svc, questionRepo := setupAnswers(t)
		about := NewAboutYouService(newFakePersonalAnswerRepository(), svc.candRepo, svc.appRepo, questionRepo)
		svc.about = about
		require.NoError(t, questionRepo.Save(ctx, &domain.ApplicationQuestion{ID: "q-old", ApplicationID: "app-1",
			Label: "Why do you want to work at Acme?", Answer: "The tooling team."}))
		_, err := about.Keep(ctx, "q-old")
		require.NoError(t, err)

		question, err := svc.AddQuestion(ctx, "app-1", "Why do you want to work with us?")

		require.NoError(t, err)
		assert.Equal(t, &domain.AnswerSuggestion{Answer: "The tooling team.", Source: domain.SuggestionKeptAnswer,
			Field: "Why do you want to work at Acme?"}, question.Suggestion)
	})
}

func TestAnswerService_SetAnswer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, questionRepo := setupAnswers(t)
	require.NoError(t, questionRepo.Save(ctx, &domain.ApplicationQuestion{ID: "q-1", ApplicationID: "app-1", Label: "Notice period"}))

	got, err := svc.SetAnswer(ctx, "q-1", "  One month\r\n")
	require.NoError(t, err)
	assert.Equal(t, "One month", got.Answer)
	assert.Equal(t, "One month", questionRepo.questions["q-1"].Answer)

	cleared, err := svc.SetAnswer(ctx, "q-1", "")
	require.NoError(t, err)
	assert.Empty(t, cleared.Answer, "an empty answer clears it")

	_, err = svc.SetAnswer(ctx, "missing-question", "x")
	assert.ErrorContains(t, err, "failed to fetch question")
}

func TestAnswerService_DeleteQuestion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, questionRepo := setupAnswers(t)
	require.NoError(t, questionRepo.Save(ctx, &domain.ApplicationQuestion{ID: "q-1", ApplicationID: "app-1", Label: "Notice period"}))

	require.NoError(t, svc.DeleteQuestion(ctx, "q-1"))

	questions, err := svc.ListByApplication(ctx, "app-1")
	require.NoError(t, err)
	assert.Empty(t, questions)
	assert.ErrorIs(t, svc.DeleteQuestion(ctx, "q-1"), ports.ErrNotFound)
}

// An answer the candidate wrote carries the flags of its text whenever it
// is read, and is never held to a letter's length
// (docs/02_features/19_draft_checks.md).
func TestAnswerService_Flags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, questionRepo := setupAnswers(t)
	require.NoError(t, questionRepo.Save(ctx, &domain.ApplicationQuestion{ID: "q-why", ApplicationID: "app-1", Label: "Why do you want to work here?"}))

	long := strings.Repeat("word ", 400)
	written, err := svc.SetAnswer(ctx, "q-why", "I am excited to apply. "+long)
	require.NoError(t, err)
	require.Len(t, written.Flags, 1, "a stock phrase, and no length flag")
	assert.Equal(t, domain.FlagStockPhrase, written.Flags[0].Kind)

	edited, err := svc.SetAnswer(ctx, "q-why", "Your post on idempotent retries is why.")
	require.NoError(t, err)
	assert.Empty(t, edited.Flags, "an edit is checked as it now reads")

	listed, err := svc.ListByApplication(ctx, "app-1")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Empty(t, listed[0].Flags)
}
