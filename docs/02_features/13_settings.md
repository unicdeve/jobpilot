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

A hosted model is a custom endpoint (ADR-0030): its base URL as the provider publishes it, with the version (`https://openrouter.ai/api/v1`), its model, and an API key chosen from the model's keys or stored there and then.
The key lives in the keychain under the source `llm`, the settings hold only its label, and a key is sent only over https or to this computer.
Wherever the model is chosen, a base URL that is not this computer says that the profile, resume and job postings are sent to its host, including from a scout running on its own.
Settings → API keys lists the model's keys as "AI model".

Search preferences are implemented as named saved searches: see
`docs/02_features/04_job_discovery.md` AC-001. They are managed from
`jobpilot-cli search saved` and the desktop's Settings → Job searches, and
one is created during onboarding. They are not optional in the way other
settings are — discovery refuses to run without one.

"Use a template" alongside them offers measured criteria to adopt and edit,
rather than a blank form (`docs/02_features/14_search_templates.md`).

Notification settings are implemented: whether to post notifications about
scout runs, and the score a strong match needs, under Settings →
Notifications and `jobpilot-cli settings notifications`
(`docs/02_features/11_notifications.md`, ADR-0028).

Prompt customization is implemented: the guidance the matching, cover
letter and written answer prompts are written under, under Settings →
Prompts and `jobpilot-cli settings prompts`
(`docs/02_features/20_prompt_customization.md`, ADR-0031).

All other categories (application mode, data retention, storage location,
browser settings, policies) remain unimplemented.
