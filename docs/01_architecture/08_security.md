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

## Candidate data

Candidate profile, resumes, application history, and generated content are local data and should not be transmitted unless required by the configured provider/integration.

## Cloud LLM warning

When a cloud LLM is configured, the UI should clearly communicate that selected context may leave the machine.

## Web prompt injection

Job descriptions and webpages must never be treated as system instructions. External text can be passed to the model only as untrusted data.
