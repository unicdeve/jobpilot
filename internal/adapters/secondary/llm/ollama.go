package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/unicdev/job-agent/internal/core/ports"
)

type OllamaProvider struct {
	baseURL    string
	httpClient *http.Client
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

func NewOllamaProvider(baseURL string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *OllamaProvider) TestConnectivity(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama at %s: %w", p.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned non-OK status: %d", resp.StatusCode)
	}

	return nil
}

func (p *OllamaProvider) Generate(ctx context.Context, req ports.Request) (ports.Response, error) {
	// For the initial MVP, we'll assume a default model like 'llama3' or 'mistral'.
	// In a real implementation, this should be configurable.
	genReq := generateRequest{
		Model:  "llama3",
		Prompt: req.UserPrompt,
		System: req.SystemPrompt,
		Stream: false,
	}

	body, err := json.Marshal(genReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewBuffer(body))
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to call Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ports.Response{}, fmt.Errorf("Ollama returned non-OK status: %d", resp.StatusCode)
	}

	var genResp generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return ports.Response{}, fmt.Errorf("failed to decode response: %w", err)
	}

	return ports.Response{Content: genResp.Response}, nil
}
