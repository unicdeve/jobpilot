package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// versionedBaseURLProviders are the providers whose base URL migration
// 00024 rewrites: the ones reached through the OpenAI-compatible adapter.
// Written out rather than read from the llm package, so the migration
// keeps meaning what it meant when it was written.
var versionedBaseURLProviders = map[string]bool{"llama.cpp": true, "lmstudio": true, "custom": true}

// versionOpenAICompatBaseURL adds /v1 to a stored OpenAI-compatible base
// URL that lacks it (ADR-0030 §4).
//
// The adapter used to append /v1/chat/completions to the base URL, and now
// appends /chat/completions, as the OpenAI SDKs do, so that a hosted
// provider's base URL can be pasted as published. Adding /v1 here keeps
// every local server that worked before working. A URL that already ended
// in /v1 was being sent to /v1/v1 and failing, and is left to work now.
//
// Ollama's adapter is its own and its URL is untouched; an empty URL
// resolves to its preset's default, which carries /v1 itself.
func versionOpenAICompatBaseURL(ctx context.Context, tx *sql.Tx) error {
	var value string
	err := tx.QueryRowContext(ctx, `SELECT value FROM user_settings WHERE key = 'llm_settings'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the LLM settings: %w", err)
	}

	// Decoded field by field, so a field this migration does not know is
	// written back exactly as it was.
	var settings map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return fmt.Errorf("reading the LLM settings: %w", err)
	}
	var provider, baseURL string
	if raw, ok := settings["provider"]; ok {
		if err := json.Unmarshal(raw, &provider); err != nil {
			return fmt.Errorf("reading the LLM provider: %w", err)
		}
	}
	if raw, ok := settings["base_url"]; ok {
		if err := json.Unmarshal(raw, &baseURL); err != nil {
			return fmt.Errorf("reading the LLM base URL: %w", err)
		}
	}
	if !versionedBaseURLProviders[provider] || baseURL == "" {
		return nil
	}

	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/v1") {
		return nil
	}
	encoded, err := json.Marshal(trimmed + "/v1")
	if err != nil {
		return err
	}
	settings["base_url"] = encoded
	rewritten, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_settings SET value = ? WHERE key = 'llm_settings'`, string(rewritten))
	return err
}
