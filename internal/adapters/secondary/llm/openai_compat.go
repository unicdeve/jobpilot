package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// OpenAICompatProvider is a ports.LLMProvider for local backends that expose
// an OpenAI-compatible HTTP API, such as llama.cpp's llama-server and LM
// Studio.
type OpenAICompatProvider struct {
	name       string
	baseURL    string
	model      string
	httpClient *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionRequest deliberately has no response_format field.
//
// llama.cpp and LM Studio both advertise OpenAI's json_schema form, and
// ADR-0017 is largely the story of an advertised constraint and an
// enforced one being different things — Ollama documents `pattern` and
// does not apply it. Neither backend was available to measure, so
// ports.Request.ResponseSchema is ignored here rather than sent on a
// promise. Callers validate every reply regardless, which is what makes
// ignoring it safe.
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// NewOpenAICompatProvider constructs an OpenAICompatProvider targeting
// baseURL with the given model. name identifies the backend (e.g.
// "llama.cpp", "lmstudio") and is returned by Provider().
func NewOpenAICompatProvider(name, baseURL, model string) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		name:    name,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TestConnectivity checks that the configured endpoint is reachable via its
// OpenAI-compatible /v1/models endpoint.
func (p *OpenAICompatProvider) TestConnectivity(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/models", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to %s at %s: %w", p.name, p.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned non-OK status: %d", p.name, resp.StatusCode)
	}

	return nil
}

// Generate sends req to the backend's /v1/chat/completions endpoint and
// returns the generated content.
func (p *OpenAICompatProvider) Generate(ctx context.Context, req ports.Request) (ports.Response, error) {
	chatReq := chatCompletionRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to call %s: %w", p.name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ports.Response{}, fmt.Errorf("%s returned non-OK status: %d", p.name, resp.StatusCode)
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return ports.Response{}, fmt.Errorf("failed to decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return ports.Response{}, fmt.Errorf("%s returned no choices", p.name)
	}

	model := chatResp.Model
	if model == "" {
		model = p.model
	}

	return ports.Response{Content: chatResp.Choices[0].Message.Content, Model: model}, nil
}

// Provider identifies this backend by its configured name (e.g.
// "llama.cpp", "lmstudio", "custom").
func (p *OpenAICompatProvider) Provider() string {
	return p.name
}
