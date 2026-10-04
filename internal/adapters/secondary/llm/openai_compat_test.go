//go:build integration

package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

func TestOpenAICompatProvider_TestConnectivity(t *testing.T) {
	t.Parallel()

	t.Run("200 is healthy", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/models", r.URL.Path)
			assert.Empty(t, r.Header.Get("Authorization"), "no key configured, none sent")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := NewOpenAICompatProvider("llama.cpp", server.URL+"/v1", "some-model").TestConnectivity(t.Context())
		assert.NoError(t, err)
	})

	t.Run("the base URL is the provider's, with its version, and a trailing slash is ignored", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// As Gemini publishes it: no /v1 anywhere.
			assert.Equal(t, "/v1beta/openai/models", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := NewOpenAICompatProvider("custom", server.URL+"/v1beta/openai/", "gemini").TestConnectivity(t.Context())
		assert.NoError(t, err)
	})

	t.Run("the key is sent as a bearer token", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer sk-test-123", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := NewOpenAICompatProvider("custom", server.URL, "gpt").withKey("openai", "sk-test-123").TestConnectivity(t.Context())
		assert.NoError(t, err)
	})

	t.Run("non-200 is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		err := NewOpenAICompatProvider("llama.cpp", server.URL, "some-model").TestConnectivity(t.Context())
		assert.ErrorContains(t, err, "non-OK status")
	})
}

// ADR-0030 §5: an error says what the status means, and never repeats what
// the provider sent, which can quote the key.
func TestOpenAICompatProvider_StatusErrors(t *testing.T) {
	t.Parallel()

	const secret = "sk-proj-abcdefghijklmnop"
	tests := []struct {
		name     string
		status   int
		withKey  bool
		contains string
	}{
		{"a refused key is named by its label", http.StatusUnauthorized, true, `refused the API key "openai" (401)`},
		{"a forbidden key is named by its label", http.StatusForbidden, true, `refused the API key "openai" (403)`},
		{"a refusal without a key says one is needed", http.StatusUnauthorized, false, "it needs an API key"},
		{"nothing found names the URL and the model", http.StatusNotFound, true, `for the model "gpt-5" (404)`},
		{"rate limiting says so", http.StatusTooManyRequests, true, "limiting requests"},
		{"anything else gives the status", http.StatusBadGateway, true, "non-OK status: 502"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				// What OpenAI sends for a wrong key: part of the key itself.
				_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided: sk-proj-***mnop"}}`))
			}))
			defer server.Close()

			provider := NewOpenAICompatProvider("custom", server.URL, "gpt-5")
			if tt.withKey {
				provider.withKey("openai", secret)
			}

			for _, err := range []error{
				provider.TestConnectivity(t.Context()),
				func() error { _, err := provider.Generate(t.Context(), ports.Request{}); return err }(),
			} {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.contains)
				assert.NotContains(t, err.Error(), "mnop", "nothing the provider said is repeated")
				assert.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestOpenAICompatProvider_Generate(t *testing.T) {
	t.Parallel()

	t.Run("sends the expected request and decodes a valid response", func(t *testing.T) {
		t.Parallel()
		var captured chatCompletionRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/chat/completions", r.URL.Path)
			assert.Equal(t, "Bearer sk-test-123", r.Header.Get("Authorization"))
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			_ = json.NewEncoder(w).Encode(chatCompletionResponse{
				Model: "some-model",
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{{Message: chatMessage{Role: "assistant", Content: "hello"}}},
			})
		}))
		defer server.Close()

		resp, err := NewOpenAICompatProvider("custom", server.URL+"/v1", "some-model").
			withKey("openai", "sk-test-123").
			Generate(t.Context(), ports.Request{SystemPrompt: "system", UserPrompt: "user", Temperature: 0.5})

		require.NoError(t, err)
		assert.Equal(t, "hello", resp.Content)
		assert.Equal(t, "some-model", resp.Model)

		require.Len(t, captured.Messages, 2)
		assert.Equal(t, "system", captured.Messages[0].Role)
		assert.Equal(t, "system", captured.Messages[0].Content)
		assert.Equal(t, "user", captured.Messages[1].Role)
		assert.Equal(t, "user", captured.Messages[1].Content)
		assert.True(t, captured.Stream, "asked to stream; this server answers in one piece, which is read too")
	})

	t.Run("empty response model falls back to the requested model", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{{Message: chatMessage{Content: "hi"}}},
			})
		}))
		defer server.Close()

		resp, err := NewOpenAICompatProvider("llama.cpp", server.URL, "requested-model").Generate(t.Context(), ports.Request{})

		require.NoError(t, err)
		assert.Equal(t, "requested-model", resp.Model)
	})

	t.Run("empty choices is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(chatCompletionResponse{})
		}))
		defer server.Close()

		_, err := NewOpenAICompatProvider("llama.cpp", server.URL, "some-model").Generate(t.Context(), ports.Request{})
		assert.ErrorContains(t, err, "no choices")
	})

	t.Run("non-200 is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		_, err := NewOpenAICompatProvider("llama.cpp", server.URL, "some-model").Generate(t.Context(), ports.Request{})
		assert.ErrorContains(t, err, "non-OK status")
	})

	t.Run("malformed response body is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer server.Close()

		_, err := NewOpenAICompatProvider("llama.cpp", server.URL, "some-model").Generate(t.Context(), ports.Request{})
		assert.ErrorContains(t, err, "failed to decode response")
	})
}

func TestOpenAICompatProvider_Provider(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "lmstudio", NewOpenAICompatProvider("lmstudio", "http://localhost:1234", "some-model").Provider())
}

// ADR-0030 §5: an error a provider reports mid-stream is repeated only when
// no key was sent, so there is none it could quote.
func TestOpenAICompatProvider_StreamErrorWithAKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"key sk-***mnop has no credit\"}}\n\n"))
	}))
	defer server.Close()

	_, err := NewOpenAICompatProvider("custom", server.URL, "m").withKey("openai", "sk-abcdmnop").
		Generate(t.Context(), ports.Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed while writing")
	assert.NotContains(t, err.Error(), "mnop")

	_, err = NewOpenAICompatProvider("llama.cpp", server.URL, "m").Generate(t.Context(), ports.Request{})
	assert.ErrorContains(t, err, "llama.cpp failed while writing: key sk-***mnop has no credit",
		"a local server sent no key, and what it said is kept")
}

func TestOpenAICompatProvider_NamesACustomEndpointByItsHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")

	err := NewOpenAICompatProvider("custom", server.URL+"/v1", "m").withKey("work", "sk").TestConnectivity(t.Context())
	assert.ErrorContains(t, err, host+` refused the API key "work" (401)`)

	err = NewOpenAICompatProvider("lmstudio", server.URL+"/v1", "m").TestConnectivity(t.Context())
	assert.ErrorContains(t, err, "lmstudio refused the request (401)")
}
