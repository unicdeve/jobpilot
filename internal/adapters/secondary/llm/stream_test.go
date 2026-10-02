//go:build integration

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// stall holds a handler until the client hangs up. The body is read first:
// a server notices a client hanging up only once it has the whole request.
func stall(r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	<-r.Context().Done()
}

// testPatience is short enough for a test to outlast, and long enough that
// a busy machine does not trip it between pieces sent 20ms apart.
var testPatience = patience{firstByte: 300 * time.Millisecond, idle: 300 * time.Millisecond}

// ollamaServer streams pieces of text pause apart, then says it is done
// unless cut is set. It stalls for good after stallAfter pieces when that is
// not negative.
func ollamaServer(t *testing.T, pieces int, pause time.Duration, stallAfter int, cut bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		for i := 0; i < pieces; i++ {
			if i == stallAfter {
				stall(r)
				return
			}
			_ = json.NewEncoder(w).Encode(generateResponse{Model: "deepseek-r1", Response: fmt.Sprint(i % 10)})
			w.(http.Flusher).Flush()
			time.Sleep(pause)
		}
		if !cut {
			_ = json.NewEncoder(w).Encode(generateResponse{Model: "deepseek-r1", Done: true})
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func ollamaWith(url string) *OllamaProvider {
	p := NewOllamaProvider(url, "deepseek-r1")
	p.patience = testPatience
	return p
}

// docs/02_features/19_draft_checks.md AC-008.
func TestOllamaProvider_Generate_Streaming(t *testing.T) {
	t.Parallel()

	t.Run("a model that keeps writing has as long as it needs", func(t *testing.T) {
		t.Parallel()
		// 40 pieces 20ms apart: well past the patience, never quiet for it.
		server := ollamaServer(t, 40, 20*time.Millisecond, -1, false)

		resp, err := ollamaWith(server.URL).Generate(t.Context(), ports.Request{})

		require.NoError(t, err)
		assert.Len(t, resp.Content, 40)
		assert.Equal(t, "deepseek-r1", resp.Model)
	})

	t.Run("a model that stops writing is given up on, saying so", func(t *testing.T) {
		t.Parallel()
		server := ollamaServer(t, 10, time.Millisecond, 3, false)

		_, err := ollamaWith(server.URL).Generate(t.Context(), ports.Request{})

		require.ErrorIs(t, err, errStalled)
		assert.ErrorContains(t, err, "Ollama stopped sending anything for 300ms")
	})

	t.Run("a server that never starts is given up on, saying so", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			stall(r)
		}))
		t.Cleanup(server.Close)

		_, err := ollamaWith(server.URL).Generate(t.Context(), ports.Request{})

		require.ErrorIs(t, err, errStalled)
		assert.ErrorContains(t, err, "Ollama sent nothing for 300ms")
	})

	t.Run("a stream cut off before the model finished is an error", func(t *testing.T) {
		t.Parallel()
		server := ollamaServer(t, 3, time.Millisecond, -1, true)

		_, err := ollamaWith(server.URL).Generate(t.Context(), ports.Request{})

		assert.ErrorContains(t, err, "ended before the model finished")
	})

	t.Run("a failure part-way through is an error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(generateResponse{Response: "Dear"})
			_ = json.NewEncoder(w).Encode(generateResponse{Error: "model runner has unexpectedly stopped"})
		}))
		t.Cleanup(server.Close)

		_, err := ollamaWith(server.URL).Generate(t.Context(), ports.Request{})

		assert.ErrorContains(t, err, "ollama failed while writing: model runner has unexpectedly stopped")
	})

	t.Run("the caller cancelling is not a stall", func(t *testing.T) {
		t.Parallel()
		server := ollamaServer(t, 10, time.Millisecond, 1, false)
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(50*time.Millisecond, cancel)

		_, err := ollamaWith(server.URL).Generate(ctx, ports.Request{})

		require.Error(t, err)
		assert.False(t, errors.Is(err, errStalled))
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// sseServer streams a completion as Server-Sent Events, pause apart, ending
// with [DONE] unless cut is set, and stalling after stallAfter events when
// that is not negative.
func sseServer(t *testing.T, pieces int, pause time.Duration, stallAfter int, cut bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		for i := 0; i < pieces; i++ {
			if i == stallAfter {
				stall(r)
				return
			}
			_, _ = fmt.Fprintf(w, "data: {\"model\":\"qwen3\",\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", fmt.Sprint(i%10))
			w.(http.Flusher).Flush()
			time.Sleep(pause)
		}
		if !cut {
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func openAICompatWith(url string) *OpenAICompatProvider {
	p := NewOpenAICompatProvider("llama.cpp", url, "requested-model")
	p.patience = testPatience
	return p
}

// docs/02_features/19_draft_checks.md AC-008.
func TestOpenAICompatProvider_Generate_Streaming(t *testing.T) {
	t.Parallel()

	t.Run("a model that keeps writing has as long as it needs", func(t *testing.T) {
		t.Parallel()
		server := sseServer(t, 40, 20*time.Millisecond, -1, false)

		resp, err := openAICompatWith(server.URL).Generate(t.Context(), ports.Request{})

		require.NoError(t, err)
		assert.Len(t, resp.Content, 40)
		assert.Equal(t, "qwen3", resp.Model)
	})

	t.Run("a model that stops writing is given up on, saying so", func(t *testing.T) {
		t.Parallel()
		server := sseServer(t, 10, time.Millisecond, 3, false)

		_, err := openAICompatWith(server.URL).Generate(t.Context(), ports.Request{})

		require.ErrorIs(t, err, errStalled)
		assert.ErrorContains(t, err, "llama.cpp stopped sending anything for 300ms")
	})

	t.Run("a stream cut off before [DONE] is an error", func(t *testing.T) {
		t.Parallel()
		server := sseServer(t, 3, time.Millisecond, -1, true)

		_, err := openAICompatWith(server.URL).Generate(t.Context(), ports.Request{})

		assert.ErrorContains(t, err, "ended before the model finished")
	})
}
