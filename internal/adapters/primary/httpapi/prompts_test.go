package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

func TestServer_PromptGuidance(t *testing.T) {
	client, _, _ := testServer(t)
	ctx := context.Background()

	prompts, err := client.ListPromptGuidance(ctx)
	require.NoError(t, err)
	require.Len(t, prompts, 3)
	for _, prompt := range prompts {
		assert.True(t, prompt.IsDefault(), prompt.Key)
		assert.NotEmpty(t, prompt.Default, prompt.Key)
	}

	saved, err := client.SavePromptGuidance(ctx, domain.PromptCoverLetter, "Open with the work.")
	require.NoError(t, err)
	assert.Equal(t, "Open with the work.", saved.Custom)
	prompts, err = client.ListPromptGuidance(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Open with the work.", prompts[1].Text())

	reset, err := client.ResetPromptGuidance(ctx, domain.PromptCoverLetter)
	require.NoError(t, err)
	assert.True(t, reset.IsDefault())

	status, message := requestError(t, client, http.MethodPut, "/v1/settings/prompts/match", map[string]any{"text": "  "})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, message, "reset the prompt")

	status, _ = requestError(t, client, http.MethodPut, "/v1/settings/prompts/apply_control", map[string]any{"text": "Pick the first link."})
	assert.Equal(t, http.StatusNotFound, status, "an extraction prompt is not one the user may change")
	status, _ = requestError(t, client, http.MethodDelete, "/v1/settings/prompts/apply_control", nil)
	assert.Equal(t, http.StatusNotFound, status)
}
