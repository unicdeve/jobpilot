// Package llm provides ports.LLMProvider implementations for local and
// remote LLM backends.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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
	patience   patience
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

// generateResponse is one line of a streamed generation: a piece of the
// text, until the last, which says it is done. A failure part-way through
// arrives as a line with an error.
type generateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// NewOllamaProvider constructs an OllamaProvider targeting baseURL. If model
// is empty, DefaultModel is used.
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	if model == "" {
		model = DefaultModel
	}
	return &OllamaProvider{
		baseURL:    baseURL,
		model:      model,
		httpClient: newHTTPClient(),
		patience:   defaultPatience,
	}
}

// TestConnectivity checks that the configured Ollama instance is reachable.
func (p *OllamaProvider) TestConnectivity(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
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
// generated content. It reads the text as the model writes it, for as long
// as the model keeps writing (docs/02_features/19_draft_checks.md D4).
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
		Stream:  true,
		Format:  req.ResponseSchema,
		Options: options,
	}

	body, err := json.Marshal(genReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	watch, stop := watchStall(ctx, p.patience)
	defer stop()

	httpReq, err := http.NewRequestWithContext(watch.ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewBuffer(body))
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.Response{}, watch.explain("Ollama", fmt.Errorf("failed to call Ollama: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ports.Response{}, fmt.Errorf("ollama returned non-OK status: %d", resp.StatusCode)
	}

	response, err := readGenerateStream(watch.body(resp.Body))
	if err != nil {
		return ports.Response{}, watch.explain("Ollama", err)
	}
	return response, nil
}

// readGenerateStream joins the lines of a streamed generation. It needs the
// line that says the model is done: a stream that ends without it was cut
// off, and part of a letter is not a letter.
func readGenerateStream(body io.Reader) (ports.Response, error) {
	var text bytes.Buffer
	var model string
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var part generateResponse
		if err := json.Unmarshal(line, &part); err != nil {
			return ports.Response{}, fmt.Errorf("failed to decode response: %w", err)
		}
		if part.Error != "" {
			return ports.Response{}, fmt.Errorf("ollama failed while writing: %s", part.Error)
		}
		text.WriteString(part.Response)
		if part.Model != "" {
			model = part.Model
		}
		if part.Done {
			return ports.Response{Content: text.String(), Model: model}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return ports.Response{}, fmt.Errorf("failed to read response: %w", err)
	}
	return ports.Response{}, errors.New("ollama's response ended before the model finished")
}

// Provider identifies this backend as "ollama".
func (p *OllamaProvider) Provider() string {
	return "ollama"
}
