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

func TestNewOllamaProvider(t *testing.T) {
	t.Parallel()

	t.Run("empty model defaults to DefaultModel", func(t *testing.T) {
		t.Parallel()
		p := NewOllamaProvider("http://localhost:11434", "")
		assert.Equal(t, DefaultModel, p.model)
	})

	t.Run("explicit model is kept", func(t *testing.T) {
		t.Parallel()
		p := NewOllamaProvider("http://localhost:11434", "mistral")
		assert.Equal(t, "mistral", p.model)
	})
}

func TestOllamaProvider_TestConnectivity(t *testing.T) {
	t.Parallel()

	t.Run("200 is healthy", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/tags", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := NewOllamaProvider(server.URL, "llama3").TestConnectivity(t.Context())
		assert.NoError(t, err)
	})

	t.Run("non-200 is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		err := NewOllamaProvider(server.URL, "llama3").TestConnectivity(t.Context())
		assert.ErrorContains(t, err, "non-OK status")
	})
}

func TestOllamaProvider_Generate(t *testing.T) {
	t.Parallel()

	t.Run("sends the expected request and decodes a valid response", func(t *testing.T) {
		t.Parallel()
		var captured generateRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/generate", r.URL.Path)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			_ = json.NewEncoder(w).Encode(generateResponse{Model: "llama3", Response: "hello"})
		}))
		defer server.Close()

		resp, err := NewOllamaProvider(server.URL, "llama3").Generate(t.Context(), ports.Request{
			SystemPrompt: "system", UserPrompt: "user", Temperature: 0.5, MaxTokens: 100,
		})

		require.NoError(t, err)
		assert.Equal(t, "hello", resp.Content)
		assert.Equal(t, "llama3", resp.Model)

		assert.Equal(t, "llama3", captured.Model)
		assert.Equal(t, "user", captured.Prompt)
		assert.Equal(t, "system", captured.System)
		assert.False(t, captured.Stream)
		require.NotNil(t, captured.Options)
		assert.InDelta(t, 0.5, captured.Options.Temperature, 0.0001)
		assert.Equal(t, 100, captured.Options.NumPredict)
	})

	t.Run("non-200 is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		_, err := NewOllamaProvider(server.URL, "llama3").Generate(t.Context(), ports.Request{})
		assert.ErrorContains(t, err, "non-OK status")
	})

	t.Run("malformed response body is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer server.Close()

		_, err := NewOllamaProvider(server.URL, "llama3").Generate(t.Context(), ports.Request{})
		assert.ErrorContains(t, err, "failed to decode response")
	})
}

func TestOllamaProvider_Provider(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "ollama", NewOllamaProvider("http://localhost:11434", "llama3").Provider())
}
