# Feature: Settings

Settings persist locally and require no hosted application account.
Saving model or prompt settings takes effect on the running daemon without a restart.

## Implemented categories

| Category | Desktop | CLI / specification |
| --- | --- | --- |
| Appearance | System, light or dark theme | Desktop-local preference |
| AI model | Provider, base URL, model and optional credential label | `settings show`, `settings set-llm`; [LLM architecture](../01_architecture/05_llm.md) |
| Prompts | Guidance for matching | `settings prompts`; [prompt customization](20_prompt_customization.md) |
| Job searches | Saved searches, templates, launch refresh and watched companies | `search saved`, `search templates`, `company`; [discovery](04_job_discovery.md) |
| Notifications | Posting toggle and strong-match score | `settings notifications`; [notifications](11_notifications.md) |
| API keys | Named credentials, without displaying stored values | `credentials`; [ADR-0019](../09_decisions/0019-storing-a-credential.md) |
| Data | Deletion previews and confirmed cleanup | `data`; [data deletion](15_data_deletion.md) |
| About | App and daemon versions | `--version` on the respective binaries |

Only one model configuration is active at a time.
Ollama, llama.cpp and LM Studio are local presets; Custom accepts an OpenAI-compatible endpoint.
A hosted model uses its published base URL, model name and optional API key ([ADR-0030](../09_decisions/0030-a-cloud-model.md)).
The key is stored in the OS keystore under source `llm`; settings retain only its label.
Keys are sent only over HTTPS or to this computer.
A nonlocal base URL shows that candidate profile, resume and job content are sent to that host, including model calls in a Scout.

Prompt customization changes guidance, not the code-owned safety frame or response contract.
Saved-search criteria cannot be empty; onboarding configures at least one search.

## Remaining categories

Retention policy, storage-location changes, browser configuration and additional policies remain unimplemented.
Scout eligibility is configured on each Scout and its saved search.
These are distinct from a global settings category.

## Acceptance Criteria

### AC-001
Settings persist locally.

### AC-002
Changing a setting does not require a hosted account.

### AC-003
Invalid configuration produces actionable validation errors.
