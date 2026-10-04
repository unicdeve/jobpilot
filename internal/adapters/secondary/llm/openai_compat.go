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
	"net/url"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// OpenAICompatProvider is a ports.LLMProvider for backends that expose an
// OpenAI-compatible HTTP API: llama.cpp's llama-server and LM Studio
// locally, and hosted models reached with an API key (ADR-0030).
type OpenAICompatProvider struct {
	name string
	// display is how errors name the backend: its provider for a known
	// server, and its host for a custom endpoint, since "custom refused
	// the API key" names nothing the user chose.
	display    string
	baseURL    string
	model      string
	key        apiKey
	httpClient *http.Client
	patience   patience
}

// apiKey is the key sent with every request, and the label it is stored
// under, which is what an error names. A zero apiKey sends none.
type apiKey struct {
	label  string
	secret string
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
//
// baseURL is the provider's, as it publishes it: with its version, such as
// https://api.openai.com/v1, for the paths to be appended to as the OpenAI
// SDKs append them (ADR-0030 §4).
func NewOpenAICompatProvider(name, baseURL, model string) *OpenAICompatProvider {
	display := name
	if parsed, err := url.Parse(baseURL); err == nil && name == ProviderCustom && parsed.Host != "" {
		display = parsed.Host
	}
	return &OpenAICompatProvider{
		name:       name,
		display:    display,
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: newHTTPClient(),
		patience:   defaultPatience,
	}
}

// withKey sends secret, stored under label, with every request.
func (p *OpenAICompatProvider) withKey(label, secret string) *OpenAICompatProvider {
	p.key = apiKey{label: label, secret: secret}
	return p
}

// newRequest builds a request to path under the base URL, carrying the key
// when there is one.
func (p *OpenAICompatProvider) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if p.key.secret != "" {
		req.Header.Set("Authorization", "Bearer "+p.key.secret)
	}
	return req, nil
}

// statusError says what a refusal means, from its status alone.
//
// The response body is never read into it. A provider's error can quote the
// key it refused — OpenAI's quotes its first and last characters — and no
// part of a key may reach an error or a log line (ADR-0019 §7, ADR-0030 §5).
func (p *OpenAICompatProvider) statusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		if p.key.label == "" {
			return fmt.Errorf("%s refused the request (%d): it needs an API key", p.display, status)
		}
		return fmt.Errorf("%s refused the API key %q (%d)", p.display, p.key.label, status)
	case http.StatusNotFound:
		return fmt.Errorf("%s found nothing at %s for the model %q (404): check the base URL and the model's name", p.display, p.baseURL, p.model)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%s is limiting requests, or the account's quota is spent (429)", p.display)
	default:
		return fmt.Errorf("%s returned non-OK status: %d", p.display, status)
	}
}

// TestConnectivity checks that the configured endpoint is reachable, and
// accepts the key, via its OpenAI-compatible models endpoint.
func (p *OpenAICompatProvider) TestConnectivity(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	req, err := p.newRequest(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to %s at %s: %w", p.display, p.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return p.statusError(resp.StatusCode)
	}

	return nil
}

// Generate sends req to the backend's chat completions endpoint and
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

	httpReq, err := p.newRequest(watch.ctx, http.MethodPost, "/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return ports.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.Response{}, watch.explain(p.display, fmt.Errorf("failed to call %s: %w", p.display, err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ports.Response{}, p.statusError(resp.StatusCode)
	}

	stream := watch.body(resp.Body)
	var response ports.Response
	if mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mediaType == "text/event-stream" {
		response, err = p.readEventStream(stream)
	} else {
		response, err = p.readCompletion(stream)
	}
	if err != nil {
		return ports.Response{}, watch.explain(p.display, err)
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
			// What the provider said is kept only when no key was sent, so
			// there is none it could quote (ADR-0030 §5).
			if p.key.secret != "" {
				return ports.Response{}, fmt.Errorf("%s failed while writing", p.display)
			}
			return ports.Response{}, fmt.Errorf("%s failed while writing: %s", p.display, chunk.Error.Message)
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
	return ports.Response{}, fmt.Errorf("%s's response ended before the model finished", p.display)
}

// readCompletion reads a completion sent in one piece.
func (p *OpenAICompatProvider) readCompletion(body io.Reader) (ports.Response, error) {
	var chatResp chatCompletionResponse
	if err := json.NewDecoder(body).Decode(&chatResp); err != nil {
		return ports.Response{}, fmt.Errorf("failed to decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return ports.Response{}, fmt.Errorf("%s returned no choices", p.display)
	}
	return ports.Response{Content: chatResp.Choices[0].Message.Content, Model: chatResp.Model}, nil
}

// Provider identifies this backend by its configured name (e.g.
// "llama.cpp", "lmstudio", "custom").
func (p *OpenAICompatProvider) Provider() string {
	return p.name
}
