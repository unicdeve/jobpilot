// Package llm provides ports.LLMProvider implementations for local and
// remote LLM backends.
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

// DefaultModel is used when OllamaProvider is constructed without an
// explicit model name.
const DefaultModel = "llama3"

// OllamaProvider is a ports.LLMProvider backed by a local Ollama instance.
type OllamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type generateOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system"`
	Stream bool   `json:"stream"`
	// Format carries a JSON Schema that Ollama compiles to a sampling
	// grammar, so the model cannot emit anything outside the shape.
	//
	// Measured against Ollama 0.34 with llama3.2:1b (ADR-0017): object
	// shape, field types and required fields are enforced across 30 runs.
	// The keywords ports.Schema cannot express are the ones that are not.
	Format  *ports.Schema    `json:"format,omitempty"`
	Options *generateOptions `json:"options,omitempty"`
}

type generateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
}

// NewOllamaProvider constructs an OllamaProvider targeting baseURL. If model
// is empty, DefaultModel is used.
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if model == "" {
		model = DefaultModel
	}
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TestConnectivity checks that the configured Ollama instance is reachable.
func (p *OllamaProvider) TestConnectivity(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama at %s: %w", p.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}

// Generate sends req to Ollama's /api/generate endpoint and returns the
// generated content.
func (p *OllamaProvider) Generate(ctx context.Context, req ports.Request) (ports.Response, error) {
	var options *generateOptions
	if req.Temperature != 0 || req.MaxTokens != 0 {
		options = &generateOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		}
	}

	genReq := generateRequest{
		Model:   p.model,
		Prompt:  req.UserPrompt,
		System:  req.SystemPrompt,
		Stream:  false,
		Format:  req.ResponseSchema,
		Options: options,
	}

	body, err := json.Marshal(genReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewBuffer(body))
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to call Ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ports.Response{}, fmt.Errorf("ollama returned non-OK status: %d", resp.StatusCode)
	}

	var genResp generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return ports.Response{}, fmt.Errorf("failed to decode response: %w", err)
	}

	return ports.Response{Content: genResp.Response, Model: genResp.Model}, nil
}

// Provider identifies this backend as "ollama".
func (p *OllamaProvider) Provider() string {
	return "ollama"
}
