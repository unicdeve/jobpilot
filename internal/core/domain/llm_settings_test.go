package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLoopbackHost(t *testing.T) {
	t.Parallel()

	for host, want := range map[string]bool{
		"localhost":        true,
		"LOCALHOST":        true,
		"localhost.":       true,
		"app.localhost":    true,
		"127.0.0.1":        true,
		"127.1.2.3":        true,
		"::1":              true,
		"[::1]":            true,
		"0.0.0.0":          false,
		"192.168.1.10":     false,
		"api.openai.com":   false,
		"localhost.evil":   false,
		"127.0.0.1.nip.io": false,
		"":                 false,
	} {
		assert.Equal(t, want, IsLoopbackHost(host), host)
	}
}

func TestLLMSettings_HostedAt(t *testing.T) {
	t.Parallel()

	for baseURL, want := range map[string]string{
		"https://api.openai.com/v1":    "api.openai.com",
		"https://openrouter.ai/api/v1": "openrouter.ai",
		"http://192.168.1.20:8080/v1":  "192.168.1.20",
		"http://localhost:11434":       "",
		"http://127.0.0.1:8080/v1":     "",
		"http://[::1]:1234/v1":         "",
		"":                             "",
		"not a url":                    "",
	} {
		assert.Equal(t, want, LLMSettings{BaseURL: baseURL}.HostedAt(), baseURL)
	}
}

func TestLLMSettings_APIKeyRef(t *testing.T) {
	t.Parallel()

	_, ok := LLMSettings{}.APIKeyRef()
	assert.False(t, ok, "no label is no key")

	ref, ok := LLMSettings{APIKeyLabel: "openrouter"}.APIKeyRef()
	assert.True(t, ok)
	assert.Equal(t, "llm/openrouter", ref.String(), "the model's keys are under their own source")
}
