# LLM Architecture

## Provider abstraction

The application must not couple domain logic directly to Ollama.

Use a provider interface such as:

```go
type Provider interface {
    Generate(ctx context.Context, request Request) (Response, error)
}
```

The exact interface should be refined during implementation.

## Initial provider

Ollama.

## Future providers

Cloud providers may be added through adapters.

The product intentionally keeps a single active provider configuration for the initial version.

## Structured output

Prefer schema-constrained structured output where the provider supports it.

Validate every model response before it reaches domain logic.

## Grounding

Candidate facts should come from structured profile/resume data.

The LLM may generate prose but must not invent factual candidate information.

## Prompt customization

Users can customize supported system prompts. Product prompts should have documented defaults and safe boundaries.
