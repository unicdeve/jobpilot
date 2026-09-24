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

Both are implemented by ADR-0017, which also records what a schema is and is not worth.

`ports.Request` carries an optional `ResponseSchema`.
Ollama compiles it to a sampling grammar; the OpenAI-compatible backends ignore it until each has been measured, so a request there behaves as it always has.

`ports.Schema` deliberately cannot express `enum`, `pattern`, `minimum` or `maximum`.
`pattern` and the numeric bounds are not enforced by a grammar at all, so writing them would promise something untrue.
`enum` is enforced, and that is the reason to avoid it: constrained to the six link kinds the profile has, a model answered "linkedin" for a Facebook link, where unconstrained it answers "facebook" and the existing guard drops it.
A schema states shape; meaning is checked after decoding, by code.

Validation is therefore not optional or conditional.
It runs on every structured reply whether or not a schema was applied, because three of the four supported backends will not have applied one.

## Grounding

Candidate facts should come from structured profile/resume data.

The LLM may generate prose but must not invent factual candidate information.

## Prompt customization

Users can customize supported system prompts. Product prompts should have documented defaults and safe boundaries.
