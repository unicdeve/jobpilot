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
configuration is supported at a time. Saved settings apply to the
running daemon immediately. The desktop's Settings screen offers the same
presets and fills in the selected provider's default base URL.

Search preferences are implemented as named saved searches: see
`docs/02_features/04_job_discovery.md` AC-001. They are managed from
`jobpilot-cli search saved` and the desktop's Settings → Job searches, and
one is created during onboarding. They are not optional in the way other
settings are — discovery refuses to run without one.

"Use a template" alongside them offers measured criteria to adopt and edit,
rather than a blank form (`docs/02_features/14_search_templates.md`).

All other categories (prompt customization, application mode, notification
settings, data retention, storage location, browser settings, policies)
remain unimplemented.
