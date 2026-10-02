package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"

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
	patience   patience
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

// chatCompletionChunk is one event of a streamed completion: a piece of the
// text in its delta.
type chatCompletionChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta chatMessage `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewOpenAICompatProvider constructs an OpenAICompatProvider targeting
// baseURL with the given model. name identifies the backend (e.g.
// "llama.cpp", "lmstudio") and is returned by Provider().
func NewOpenAICompatProvider(name, baseURL, model string) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		name:       name,
		baseURL:    baseURL,
		model:      model,
		httpClient: newHTTPClient(),
		patience:   defaultPatience,
	}
}

// TestConnectivity checks that the configured endpoint is reachable via its
// OpenAI-compatible /v1/models endpoint.
func (p *OpenAICompatProvider) TestConnectivity(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
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
// returns the generated content. It asks for the text as the model writes
// it and waits for as long as the model keeps writing
// (docs/02_features/19_draft_checks.md D4); a server that answers in one
// piece instead is read that way.
func (p *OpenAICompatProvider) Generate(ctx context.Context, req ports.Request) (ports.Response, error) {
	chatReq := chatCompletionRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	watch, stop := watchStall(ctx, p.patience)
	defer stop()

	httpReq, err := http.NewRequestWithContext(watch.ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return ports.Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.Response{}, watch.explain(p.name, fmt.Errorf("failed to call %s: %w", p.name, err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ports.Response{}, fmt.Errorf("%s returned non-OK status: %d", p.name, resp.StatusCode)
	}

	stream := watch.body(resp.Body)
	var response ports.Response
	if mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mediaType == "text/event-stream" {
		response, err = p.readEventStream(stream)
	} else {
		response, err = p.readCompletion(stream)
	}
	if err != nil {
		return ports.Response{}, watch.explain(p.name, err)
	}
	if response.Model == "" {
		response.Model = p.model
	}
	return response, nil
}

// readEventStream joins the deltas of a streamed completion. It needs the
// closing [DONE]: a stream that ends without it was cut off, and part of a
// letter is not a letter.
func (p *OpenAICompatProvider) readEventStream(body io.Reader) (ports.Response, error) {
	var text bytes.Buffer
	var model string
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		data, ok := bytes.CutPrefix(bytes.TrimSpace(scanner.Bytes()), []byte("data:"))
		if !ok {
			continue // a blank line between events, or a comment
		}
		data = bytes.TrimSpace(data)
		if string(data) == "[DONE]" {
			return ports.Response{Content: text.String(), Model: model}, nil
		}
		var chunk chatCompletionChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return ports.Response{}, fmt.Errorf("failed to decode response: %w", err)
		}
		if chunk.Error != nil {
			return ports.Response{}, fmt.Errorf("%s failed while writing: %s", p.name, chunk.Error.Message)
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		for _, choice := range chunk.Choices {
			text.WriteString(choice.Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return ports.Response{}, fmt.Errorf("failed to read response: %w", err)
	}
	return ports.Response{}, fmt.Errorf("%s's response ended before the model finished", p.name)
}

// readCompletion reads a completion sent in one piece.
func (p *OpenAICompatProvider) readCompletion(body io.Reader) (ports.Response, error) {
	var chatResp chatCompletionResponse
	if err := json.NewDecoder(body).Decode(&chatResp); err != nil {
		return ports.Response{}, fmt.Errorf("failed to decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return ports.Response{}, fmt.Errorf("%s returned no choices", p.name)
	}
	return ports.Response{Content: chatResp.Choices[0].Message.Content, Model: chatResp.Model}, nil
}

// Provider identifies this backend by its configured name (e.g.
// "llama.cpp", "lmstudio", "custom").
func (p *OpenAICompatProvider) Provider() string {
	return p.name
}
