# Feature: Settings

## Categories

- LLM provider/model
- prompt customization
- search preferences
- application mode
- notification settings
- data retention
- storage location where supported
- browser settings post-MVP
- policies post-MVP

## Acceptance Criteria

### AC-001
Settings persist locally.

### AC-002
Changing a setting does not require a hosted account.

### AC-003
Invalid configuration produces actionable validation errors.

## Implementation status

LLM provider/model is implemented via `jobpilot-cli settings show` /
`settings set-llm` (see `docs/CLI.md`), persisted locally in SQLite's
`user_settings` table. Known local providers (Ollama, llama.cpp, LM Studio)
are offered as presets with a manually-entered model name; a "Custom"
option accepts any OpenAI-compatible endpoint. Per
`docs/01_architecture/05_llm.md`, only a single active provider
configuration is supported at a time.

Search preferences are implemented as named saved searches: see
`docs/02_features/04_job_discovery.md` AC-001. They are managed from
`jobpilot-cli search saved` and the desktop's Settings → Job searches, and
one is created during onboarding. They are not optional in the way other
settings are — discovery refuses to run without one.

All other categories (prompt customization, application mode, notification
settings, data retention, storage location, browser settings, policies)
remain unimplemented.
