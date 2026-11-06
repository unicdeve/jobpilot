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
		require.Len(t, prompts, 1, "matching is the one prompt left since ADR-0034")
		for _, prompt := range prompts {
			assert.True(t, prompt.IsDefault(), prompt.Key)
		}
	})

	t.Run("saved guidance is kept, trimmed, and read back", func(t *testing.T) {
		t.Parallel()
		svc := NewSettingsService(newFakeSettingsRepository())

		saved, err := svc.SavePromptGuidance(ctx, domain.PromptMatch, "  Weigh remote work heavily.\n")
		require.NoError(t, err)
		assert.Equal(t, "Weigh remote work heavily.", saved.Custom)

		got, err := svc.PromptGuidance(ctx, domain.PromptMatch)
		require.NoError(t, err)
		assert.Equal(t, "Weigh remote work heavily.", got.Text())
		assert.False(t, got.DefaultChanged)
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
		repo.values[promptGuidanceKey] = `{"match":{"text":"Weigh remote work.","default_digest":"an older default"}}`
		svc := NewSettingsService(repo)

		got, err := svc.PromptGuidance(ctx, domain.PromptMatch)
		require.NoError(t, err)
		assert.True(t, got.DefaultChanged)

		saved, err := svc.SavePromptGuidance(ctx, domain.PromptMatch, "Weigh remote work.")
		require.NoError(t, err)
		assert.False(t, saved.DefaultChanged, "saving again records the default the user was shown")
	})

	t.Run("a key that is not a prompt is not found, the removed ones included", func(t *testing.T) {
		t.Parallel()
		svc := NewSettingsService(newFakeSettingsRepository())

		for _, key := range []domain.PromptKey{"apply_control", "cover_letter", "written_answer"} {
			_, err := svc.SavePromptGuidance(ctx, key, "Pick the first link.")
			assert.ErrorIs(t, err, ports.ErrNotFound, key)
			_, err = svc.ResetPromptGuidance(ctx, key)
			assert.ErrorIs(t, err, ports.ErrNotFound, key)
			_, err = svc.PromptGuidance(ctx, key)
			assert.ErrorIs(t, err, ports.ErrNotFound, key)
		}
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
