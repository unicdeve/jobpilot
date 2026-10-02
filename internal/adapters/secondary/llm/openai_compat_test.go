//go:build integration

package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := NewOpenAICompatProvider("llama.cpp", server.URL, "some-model").TestConnectivity(t.Context())
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

func TestOpenAICompatProvider_Generate(t *testing.T) {
	t.Parallel()

	t.Run("sends the expected request and decodes a valid response", func(t *testing.T) {
		t.Parallel()
		var captured chatCompletionRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/chat/completions", r.URL.Path)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			_ = json.NewEncoder(w).Encode(chatCompletionResponse{
				Model: "some-model",
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{{Message: chatMessage{Role: "assistant", Content: "hello"}}},
			})
		}))
		defer server.Close()

		resp, err := NewOpenAICompatProvider("llama.cpp", server.URL, "some-model").Generate(t.Context(), ports.Request{
			SystemPrompt: "system", UserPrompt: "user", Temperature: 0.5,
		})

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
