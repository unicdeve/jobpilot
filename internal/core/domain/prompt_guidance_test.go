package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptDefinitions(t *testing.T) {
	t.Parallel()

	keys := make([]PromptKey, 0, 1)
	for _, definition := range PromptDefinitions() {
		keys = append(keys, definition.Key)
		assert.NotEmpty(t, definition.Title, definition.Key)
		assert.NotEmpty(t, definition.UsedFor, definition.Key)
		_, err := NormalizeGuidance(definition.Default)
		assert.NoError(t, err, "%s's default must be guidance a user could have saved", definition.Key)
	}
	assert.Equal(t, []PromptKey{PromptMatch}, keys,
		"only the prompt that makes a judgement is offered (ADR-0031 §1, ADR-0034 §1)")

	for _, key := range []PromptKey{"apply_control", "cover_letter", "written_answer"} {
		_, ok := PromptDefinitionFor(key)
		assert.False(t, ok, "%s is not offered: an extraction prompt, or one JobPilot no longer sends", key)
	}
}

func TestGuidanceOf(t *testing.T) {
	t.Parallel()
	definition, ok := PromptDefinitionFor(PromptMatch)
	require.True(t, ok)

	t.Run("nothing saved is the default", func(t *testing.T) {
		t.Parallel()
		guidance := GuidanceOf(definition, nil)

		assert.True(t, guidance.IsDefault())
		assert.Equal(t, definition.Default, guidance.Text())
		assert.False(t, guidance.DefaultChanged)
	})

	t.Run("saved guidance replaces the default", func(t *testing.T) {
		t.Parallel()
		guidance := GuidanceOf(definition, PromptGuidanceSettings{
			PromptMatch: {Text: "Open with the work.", DefaultDigest: GuidanceDigest(definition.Default)},
		})

		assert.False(t, guidance.IsDefault())
		assert.Equal(t, "Open with the work.", guidance.Text())
		assert.Equal(t, definition.Default, guidance.Default, "the default is still shown beside it")
		assert.False(t, guidance.DefaultChanged)
	})

	t.Run("a default changed since saving is pointed out", func(t *testing.T) {
		t.Parallel()
		guidance := GuidanceOf(definition, PromptGuidanceSettings{
			PromptMatch: {Text: "Open with the work.", DefaultDigest: GuidanceDigest("an older default")},
		})

		assert.True(t, guidance.DefaultChanged)
		assert.Equal(t, "Open with the work.", guidance.Text(), "the user's guidance is kept")
	})

	t.Run("another prompt's guidance is not this one's", func(t *testing.T) {
		t.Parallel()
		guidance := GuidanceOf(definition, PromptGuidanceSettings{"cover_letter": {Text: "Open with the work."}})

		assert.True(t, guidance.IsDefault())
	})
}

func TestNormalizeGuidance(t *testing.T) {
	t.Parallel()

	got, err := NormalizeGuidance("  Open with the work.\r\nNo exclamation marks.\n\n")
	require.NoError(t, err)
	assert.Equal(t, "Open with the work.\nNo exclamation marks.", got)

	for name, text := range map[string]string{
		"empty":      "",
		"blank":      " \n\t ",
		"over limit": strings.Repeat("é", MaxGuidanceLength+1),
	} {
		_, err := NormalizeGuidance(text)
		assert.ErrorIs(t, err, ErrInvalidGuidance, name)
	}

	_, err = NormalizeGuidance(strings.Repeat("é", MaxGuidanceLength))
	assert.NoError(t, err, "the limit counts characters, not bytes")
}
