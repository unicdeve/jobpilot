# Security Architecture

## Principles

- Local-first.
- Least privilege.
- No raw password storage.
- Secrets never appear in logs.
- External content is untrusted.
- LLM output is untrusted until validated.
- Policy enforcement occurs outside the LLM.
- External side effects require explicit authorization.
- Plugins are isolated where possible.

## Secret storage

Use OS-native secure credential storage for API keys and other credentials.

ADR-0019 settles the shape: a credential is identified by a source and a
user-chosen label, held in the OS keychain, and reachable only from inside
the daemon. It goes in through the API and never comes out — there is no
route that returns a secret, because the daemon has no authentication
(ADR-0012) and a readable secret would be less protected there than in a
file.

Two things that look like secrets are deliberately not stored. A job board
login lives in the browser worker's own Chrome profile, which the user signs
into by hand. A cloud LLM credential belongs to the local provider — Ollama
holds its own — and `domain.LLMSettings` has no key field.

## Candidate data

Candidate profile, resumes, application history, and generated content are local data and should not be transmitted unless required by the configured provider/integration.

## Cloud LLM warning

When a cloud LLM is configured, the UI should clearly communicate that selected context may leave the machine.

## Web prompt injection

Job descriptions and webpages must never be treated as system instructions. External text can be passed to the model only as untrusted data.
