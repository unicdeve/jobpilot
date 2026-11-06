# LLM Architecture

## Provider abstraction

The application must not couple domain logic directly to Ollama.

Core uses `ports.LLMProvider`, whose generation method is:

```go
type LLMProvider interface {
    Generate(ctx context.Context, request Request) (Response, error)
}
```

The interface also exposes provider health, model listing and provider identity.
See `internal/core/ports/llm.go` for the complete contract.

## Initial provider

Ollama.

## Hosted models

A hosted model is reached as a custom OpenAI-compatible endpoint with an API key (ADR-0030).
The key is a credential under the source `llm` (ADR-0019), named in the settings by its label, read from the keychain by the daemon each time it builds a request, and sent as a bearer token, only over https or to this computer.

The OpenAI-compatible adapter appends `/chat/completions` and `/models` to the base URL, which includes its version as every provider publishes it; Ollama's adapter is its own and takes the server's root.
An error is written from the response status, never from the response body, which can quote the key.

Only `Authorization: Bearer` is supported.

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

## Time to write

Generation is streamed, from Ollama as lines of JSON and from OpenAI-compatible servers as Server-Sent Events, and a request is bounded by silence rather than by its total length (`docs/02_features/19_draft_checks.md` D4).
It is given up on after 60 seconds without a byte once the server has started answering, and after five minutes before it has, since a large model loading from disk sends nothing until it is ready.
The error says which, so a stalled server is not mistaken for the caller cancelling.
A connectivity check, which writes nothing, keeps a 10-second limit of its own.

A stream must end the way the backend ends a finished answer — Ollama's `done`, OpenAI's `[DONE]` — or the request fails: part of a reply is not a reply.
An OpenAI-compatible server that answers in one piece, ignoring `stream`, is read that way, by its `Content-Type`.

## Grounding

Candidate facts should come from structured profile/resume data.

The LLM scores matches, reads resumes and reads job pages; it writes nothing that goes to an employer (ADR-0034).
It must not invent factual candidate information: a score is validated, and a resume's suggestions are reviewed by the candidate before they reach the profile (ADR-0011).

The letters and answers the candidate writes carry deterministic flags for stock phrases, length and unsupported numbers of years (`docs/02_features/19_draft_checks.md`).

## Prompt customization

Users can customize supported system prompts. Product prompts should have documented defaults and safe boundaries.

ADR-0031 decides what that means.
One prompt can be changed — matching — and only its guidance: the instructions about judgement and style.
The cover letter and written answer prompts were removed by ADR-0034.
The frame, which holds the rules that protect the candidate and the reply's contract, is owned by code and comes after the guidance, saying it comes first.
The guidance is the user's own instruction, so it is sent in the system prompt and never beside a job posting.

What the guidance cannot do is enforced by code rather than by the frame: a score is validated, and a Scout's eligibility is deterministic.
The defaults are documented in `docs/02_features/20_prompt_customization.md`.
