# ADR-0030: A cloud model

## Status

Accepted on 2026-10-04, and implemented.
Two choices were made that day: the key picker offers only the model's own keys, and where the data goes is a note beside the field rather than a confirmation.

It amends ADR-0019, whose context said "the cloud LLM credential belongs to Ollama" and that `domain.LLMSettings` has no key field.
That held while every configured backend was local; it stops holding once the user can point JobPilot at a hosted model.
Everything else in ADR-0019 stands, and this builds on it: the key is a credential like any other, kept in the keychain, never returned.

## Context

The user asked for it directly: choosing "Custom" should let them type a provider's base URL and model, and choose an API key already stored or store a new one.

Four things bear on how.

### Every backend JobPilot can reach today is unauthenticated

`OpenAICompatProvider` sends no `Authorization` header.
llama.cpp, LM Studio and a local custom server do not need one; OpenAI, OpenRouter, Groq, Mistral, Together, DeepSeek, Gemini's OpenAI-compatible endpoint and Anthropic's all refuse a request without one.

### A cloud base URL already includes its version

The adapter appends `/v1/chat/completions` and `/v1/models` to the base URL.
Every hosted provider publishes its base URL with the version in it, for the OpenAI SDKs to append `/chat/completions` to:

| Provider | Published base URL |
| --- | --- |
| OpenAI | `https://api.openai.com/v1` |
| OpenRouter | `https://openrouter.ai/api/v1` |
| Groq | `https://api.groq.com/openai/v1` |
| Gemini | `https://generativelanguage.googleapis.com/v1beta/openai` |
| Anthropic | `https://api.anthropic.com/v1` |

Pasted as published, each becomes `…/v1/v1/chat/completions`, or for Gemini `…/v1beta/openai/v1/chat/completions`, and fails with a 404 the user cannot explain.

### A provider's error can carry the key

OpenAI answers a wrong key with a message that quotes the key's first and last characters.
ADR-0019 §7 forbids any part of a key in a log line or an error, and the adapter today does not put the response body in its errors — only the status — which is the safe habit to keep.

### A hosted model receives the candidate's data

Matching, letters, answers and resume reading send the profile, the resume and the job postings to the model.
With a local backend that never leaves the machine; with a hosted one, all of it goes to the provider, every time, including from a Scout running unattended.
`AGENTS.md` says user data stays local *by default*: choosing a hosted model is the user's choice to make, and they should be told what it means when they make it.

## Decision

### 1. The model's key is a credential under the source `llm`

A key is stored as ADR-0019 stores every credential: in the keychain, addressed by a source and a label, written through `PUT /v1/credentials/llm/{label}` and never read back.
The source is `llm`, owned by the code that needs it (ADR-0019 §1); the label is the user's: `openrouter`, `work`, `groq`.

`LLMSettings` gains one field:

```go
type LLMSettings struct {
	Provider    string `json:"provider"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	APIKeyLabel string `json:"api_key_label,omitempty"` // a credential under "llm"; empty for none
}
```

It holds the label, never the key.
This settles ADR-0019's open question 2 for the model: the settings say which label is used.

The key picker lists only the credentials under `llm`.
A job source's key is not offered for the model, nor the model's for a job source.

### 2. The key is read when it is used

The daemon reads the key from the keychain each time it builds a request, through `CredentialService.Get`, and holds it no longer than the request.
A key replaced under the same label is used from the next request on, as saved settings already are (`ReloadingProvider`).

A label whose key is not in the keychain fails the request with a message that names the label and says where to store it, and is reported by the readiness check (`GET /v1/settings/llm/health`) before any work starts.

### 3. A key is sent only over HTTPS, or to this machine

Saving settings with a key and an `http` base URL is refused unless the host is loopback (`localhost`, `127.0.0.0/8`, `::1`).
A local llama-server started with `--api-key` still works; a key sent across a network in the clear does not.
The rule is deterministic and lives in core, with the other settings validation.

### 4. The base URL is the provider's, as published

The OpenAI-compatible adapter appends `/chat/completions` and `/models` to the base URL, with any trailing slash removed, as the OpenAI SDKs do.
The presets' defaults become `http://localhost:8080/v1` (llama.cpp) and `http://localhost:1234/v1` (LM Studio).
Ollama's adapter is its own and does not change.

A Go migration rewrites stored settings so nothing that works today stops working: a llama.cpp, LM Studio or custom base URL gains `/v1`, unless it already ends in it.
One that already ended in `/v1` was failing with `/v1/v1`, and now works.

### 5. Errors are written from the status, never from the body

| Status | Error |
| --- | --- |
| 401, 403 | the provider refused the API key, naming the label |
| 404 | the base URL or the model was not found, naming both |
| 429 | the provider is limiting requests, or the account's quota is spent |
| other | the provider answered with that status |

The response body is never put into an error, a log line or a run's trail.
An error the provider reports partway through a stream is repeated only when no key was sent: then there is no key it could quote, and what a local server says about a failure (a prompt too long for its context, say) is worth keeping.

An error names a custom endpoint by its host, since "custom refused the API key" names nothing the user chose.

### 6. The user is told where their data goes

Wherever the model is chosen — Settings → AI model, the first-run wizard, the CLI's `settings set-llm` — a base URL whose host is not this machine shows the host, and says that the profile, resume and job postings are sent there when JobPilot reads, scores or writes, including when a Scout runs on its own.
It is a statement beside the field, not a confirmation to click through.

### 7. Choosing or adding a key where the model is chosen

Under "Custom", the form gains an API key choice: "No key", each stored `llm` credential by label, and "Add a key…".
Adding asks for a label and the key, stores it in the keychain, and selects it, with ADR-0019 §4's warning that it cannot be shown again.
Settings → API keys lists the model's keys beside the job sources', named "AI model" rather than `llm`.

The CLI's `settings set-llm` asks the same, for "Custom": an existing key, a new one (typed without echo), or none.

## Consequences

- `domain.LLMSettings` gains `api_key_label`; stored settings without it have no key, as today.
- `llm.NewProvider` takes the key, and its callers — `ReloadingProvider`, and the save and health handlers, which construct a provider directly today — get it through one function the daemon wires to `CredentialService`, so the HTTP layer never handles a key it did not need to.
- A Go migration rewrites the stored base URL for the OpenAI-compatible presets (decision 4). The desktop's preset list, which mirrors `llm.Presets`, changes with it.
- Structured output to a hosted OpenAI-compatible model is still unconstrained: ADR-0017 left `response_format` unsent until each backend is measured, and every reply is validated regardless. Measuring OpenAI's is a separate change.
- Only `Authorization: Bearer` is supported. A provider that wants another header (Azure OpenAI's `api-key`) is not.
- `docs/01_architecture/05_llm.md`'s "Future providers" becomes what is built, and `docs/02_features/13_settings.md` gains the key and the notice.

## What building it changed

- **Errors name a custom endpoint by its host.** The first version said "custom refused the API key", naming the provider's internal key rather than anything the user chose.
- **An error a stream reports is kept only without a key.** The adapter had put a mid-stream error's message into its error, unconditionally; with a key sent, that is the body decision 5 rules out.
- **The save and readiness checks go through the daemon's provider.** The HTTP handlers had each built a provider from the settings; they now ask the one every generation uses, so a key is read in one place and the HTTP layer no longer imports the LLM adapter.
- **Choosing Custom clears a preset's address.** Switching from Ollama kept `http://localhost:11434`, which is never a custom endpoint's address and hid the placeholder showing what one looks like.
- **A removed key shows as removed.** A label the settings name and the keychain no longer lists reads "(not stored)" with a warning in the desktop, and `settings show` says the same, rather than either reading as configured.

Measured in the desktop against a scratch daemon and keychain: a key stored from the model settings is selected and sent, a wrong one is refused naming its label with nothing of the provider's reply, a key over plain http to another host is refused before saving, and the notice shows for a hosted host and not for loopback.

## Alternatives considered

**Any stored credential offered for the model.** One list, but a job source's key offered as a model key is a mistake waiting to be made, and the source is how ADR-0019 says who a key is for.

**The key typed into the model settings and stored with them.** A secret in the settings table, which ADR-0019 option B rejected outright.

**A provider per vendor**, with each one's URL built in. Friendlier for the five shown above, and a list to keep current for every vendor there is, when all of them speak the same protocol at a URL the vendor publishes.

**Keep appending `/v1`, and tell the user to remove it.** No migration, and a 404 for everyone who pastes the URL their provider gave them.

**A confirmation before saving a hosted model.** Asked once and clicked through; the notice stays beside the field for as long as the setting does.
