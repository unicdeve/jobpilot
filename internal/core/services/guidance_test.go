package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

func TestSettingsService_PromptGuidance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("every prompt starts on its default", func(t *testing.T) {
		t.Parallel()
		svc := NewSettingsService(newFakeSettingsRepository())

		prompts, err := svc.ListPromptGuidance(ctx)

		require.NoError(t, err)
		require.Len(t, prompts, 3)
		for _, prompt := range prompts {
			assert.True(t, prompt.IsDefault(), prompt.Key)
		}
	})

	t.Run("saved guidance is kept, trimmed, and read back", func(t *testing.T) {
		t.Parallel()
		svc := NewSettingsService(newFakeSettingsRepository())

		saved, err := svc.SavePromptGuidance(ctx, domain.PromptCoverLetter, "  Open with the work.\n")
		require.NoError(t, err)
		assert.Equal(t, "Open with the work.", saved.Custom)

		got, err := svc.PromptGuidance(ctx, domain.PromptCoverLetter)
		require.NoError(t, err)
		assert.Equal(t, "Open with the work.", got.Text())
		assert.False(t, got.DefaultChanged)

		other, err := svc.PromptGuidance(ctx, domain.PromptWrittenAnswer)
		require.NoError(t, err)
		assert.True(t, other.IsDefault(), "saving one prompt leaves the others alone")
	})

	t.Run("reset goes back to the default", func(t *testing.T) {
		t.Parallel()
		svc := NewSettingsService(newFakeSettingsRepository())
		_, err := svc.SavePromptGuidance(ctx, domain.PromptMatch, "Weigh remote work heavily.")
		require.NoError(t, err)

		reset, err := svc.ResetPromptGuidance(ctx, domain.PromptMatch)

		require.NoError(t, err)
		assert.True(t, reset.IsDefault())
		got, err := svc.PromptGuidance(ctx, domain.PromptMatch)
		require.NoError(t, err)
		assert.True(t, got.IsDefault())
	})

	t.Run("resetting a prompt on its default writes nothing", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		svc := NewSettingsService(repo)

		_, err := svc.ResetPromptGuidance(ctx, domain.PromptMatch)

		require.NoError(t, err)
		assert.NotContains(t, repo.values, promptGuidanceKey)
	})

	t.Run("a default changed since saving is pointed out", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.values[promptGuidanceKey] = `{"cover_letter":{"text":"Open with the work.","default_digest":"an older default"}}`
		svc := NewSettingsService(repo)

		got, err := svc.PromptGuidance(ctx, domain.PromptCoverLetter)
		require.NoError(t, err)
		assert.True(t, got.DefaultChanged)

		saved, err := svc.SavePromptGuidance(ctx, domain.PromptCoverLetter, "Open with the work.")
		require.NoError(t, err)
		assert.False(t, saved.DefaultChanged, "saving again records the default the user was shown")
	})

	t.Run("a key that is not a prompt is not found", func(t *testing.T) {
		t.Parallel()
		svc := NewSettingsService(newFakeSettingsRepository())

		_, err := svc.SavePromptGuidance(ctx, "apply_control", "Pick the first link.")
		assert.ErrorIs(t, err, ports.ErrNotFound)
		_, err = svc.ResetPromptGuidance(ctx, "apply_control")
		assert.ErrorIs(t, err, ports.ErrNotFound)
		_, err = svc.PromptGuidance(ctx, "apply_control")
		assert.ErrorIs(t, err, ports.ErrNotFound)
	})

	t.Run("guidance that cannot be kept is refused, and nothing is written", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		svc := NewSettingsService(repo)

		_, err := svc.SavePromptGuidance(ctx, domain.PromptMatch, "   ")

		assert.ErrorIs(t, err, domain.ErrInvalidGuidance)
		assert.NotContains(t, repo.values, promptGuidanceKey)
	})

	t.Run("a stored null reads as nothing saved", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.values[promptGuidanceKey] = "null"
		svc := NewSettingsService(repo)

		_, err := svc.SavePromptGuidance(ctx, domain.PromptMatch, "Weigh remote work heavily.")

		require.NoError(t, err)
	})

	t.Run("a repository error is reported", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.getErr = errors.New("boom")
		svc := NewSettingsService(repo)

		_, err := svc.ListPromptGuidance(ctx)

		assert.ErrorContains(t, err, "failed to get prompt guidance")
	})
}

// assertGuided asserts the parts of a system prompt ADR-0031 §3 fixes: the
// guidance sits between the role and the rules, and the rules say they
// come first.
func assertGuided(t *testing.T, system, role, guidance, rules string) {
	t.Helper()
	roleAt := strings.Index(system, role)
	guidanceAt := strings.Index(system, guidance)
	rulesAt := strings.Index(system, rules)
	require.True(t, roleAt >= 0 && guidanceAt >= 0 && rulesAt >= 0, "role, guidance and rules must all be sent:\n%s", system)
	assert.Less(t, roleAt, guidanceAt, "the role comes before the guidance")
	assert.Less(t, guidanceAt, rulesAt, "the rules come after the guidance, and have the last word")
	assert.Contains(t, system, rulesPrecedence)
}

func TestGuidedPrompts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const custom = "Open with the hardest problem I solved. Sign off with my first name only."

	settingsWith := func(t *testing.T, key domain.PromptKey, text string) *SettingsService {
		t.Helper()
		svc := NewSettingsService(newFakeSettingsRepository())
		if text != "" {
			_, err := svc.SavePromptGuidance(ctx, key, text)
			require.NoError(t, err)
		}
		return svc
	}

	t.Run("matching", func(t *testing.T) {
		t.Parallel()
		for name, text := range map[string]string{"default": "", "custom": custom} {
			svc, _, jobRepo, candRepo, llm := newTestMatchingService(t)
			svc.WithGuidance(settingsWith(t, domain.PromptMatch, text))
			llm.response = ports.Response{Content: validMatchJSON}
			require.NoError(t, jobRepo.Save(ctx, &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1"}))
			require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))

			_, err := svc.AnalyzeJobForCandidate(ctx, "cand-1", "job-1")
			require.NoError(t, err, name)

			want := text
			if want == "" {
				want = domain.DefaultGuidance(domain.PromptMatch).Default
			}
			assertGuided(t, llm.lastRequest.SystemPrompt, matchRole, want, matchRules)
			assert.NotNil(t, llm.lastRequest.ResponseSchema, "the reply's contract is not guidance")
		}
	})

	t.Run("cover letter", func(t *testing.T) {
		t.Parallel()
		const styleLine = "- No stock openings or closings, and do not call the candidate a great fit."
		for name, text := range map[string]string{"default": "", "custom": custom} {
			svc, _, appRepo, jobRepo, candRepo, _, llm := newTestCoverLetterService(t)
			svc.WithGuidance(settingsWith(t, domain.PromptCoverLetter, text))
			llm.response = ports.Response{Content: "A letter."}
			require.NoError(t, jobRepo.Save(ctx, &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1"}))
			require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))
			require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", JobID: "job-1", CandidateID: "cand-1"}))

			_, err := svc.Generate(ctx, "app-1")
			require.NoError(t, err, name)

			sent := llm.lastRequest
			if text == "" {
				assertGuided(t, sent.SystemPrompt, letterRole, domain.DefaultGuidance(domain.PromptCoverLetter).Default, letterRules)
				assert.Contains(t, sent.UserPrompt, styleLine, "the default's style is repeated at the end")
			} else {
				assertGuided(t, sent.SystemPrompt, letterRole, custom, letterRules)
				assert.NotContains(t, sent.SystemPrompt, "I am excited to apply", "the default's style is replaced, not added to")
				assert.NotContains(t, sent.UserPrompt, styleLine, "the default's style would contradict the user's")
			}
			assert.Contains(t, sent.UserPrompt, "- Claim only what the candidate profile", "the protecting reminder is kept whatever the guidance")
			assert.Contains(t, sent.UserPrompt, "- Leave out everything listed under Never mention.")
			assert.NotContains(t, sent.UserPrompt, custom, "the user's guidance is never put beside the posting")
		}
	})

	t.Run("written answer", func(t *testing.T) {
		t.Parallel()
		const styleLine = "- Answer only the question, as briefly as it calls for, with no stock phrases."
		for name, text := range map[string]string{"default": "", "custom": custom} {
			svc, questionRepo, appRepo, jobRepo, candRepo, llm := newTestAnswerService(t)
			svc.WithGuidance(settingsWith(t, domain.PromptWrittenAnswer, text))
			llm.response = ports.Response{Content: "An answer."}
			require.NoError(t, jobRepo.Save(ctx, &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1"}))
			require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))
			require.NoError(t, appRepo.Save(ctx, &domain.Application{ID: "app-1", JobID: "job-1", CandidateID: "cand-1"}))
			require.NoError(t, questionRepo.Save(ctx, &domain.ApplicationQuestion{
				ID: "q-1", ApplicationID: "app-1", Label: "Tell us about a project you led.", Type: domain.QuestionTypeNarrative,
			}))

			_, err := svc.Resolve(ctx, "q-1")
			require.NoError(t, err, name)

			sent := llm.lastRequest
			if text == "" {
				assertGuided(t, sent.SystemPrompt, answerRole, domain.DefaultGuidance(domain.PromptWrittenAnswer).Default, answerRules)
				assert.Contains(t, sent.UserPrompt, styleLine)
			} else {
				assertGuided(t, sent.SystemPrompt, answerRole, custom, answerRules)
				assert.NotContains(t, sent.UserPrompt, styleLine)
			}
			assert.Contains(t, sent.UserPrompt, "- Claim only what the candidate profile")
			assert.NotContains(t, sent.UserPrompt, custom)
		}
	})

	t.Run("a generator built without the settings writes under the default", func(t *testing.T) {
		t.Parallel()
		svc, _, jobRepo, candRepo, llm := newTestMatchingService(t)
		llm.response = ports.Response{Content: validMatchJSON}
		require.NoError(t, jobRepo.Save(ctx, &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1"}))
		require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))

		_, err := svc.AnalyzeJobForCandidate(ctx, "cand-1", "job-1")

		require.NoError(t, err)
		assert.Contains(t, llm.lastRequest.SystemPrompt, domain.DefaultGuidance(domain.PromptMatch).Default)
	})

	t.Run("guidance that cannot be read stops the request", func(t *testing.T) {
		t.Parallel()
		repo := newFakeSettingsRepository()
		repo.getErr = errors.New("boom")
		svc, _, jobRepo, candRepo, llm := newTestMatchingService(t)
		svc.WithGuidance(NewSettingsService(repo))
		require.NoError(t, jobRepo.Save(ctx, &domain.Job{ID: "job-1", CanonicalURL: "https://acme.example/job-1"}))
		require.NoError(t, candRepo.Save(ctx, &domain.Candidate{ID: "cand-1"}))

		_, err := svc.AnalyzeJobForCandidate(ctx, "cand-1", "job-1")

		assert.ErrorContains(t, err, "failed to read the prompt's guidance")
		assert.Zero(t, llm.generateCalls, "a prompt is not sent under guidance the user did not choose")
	})
}
