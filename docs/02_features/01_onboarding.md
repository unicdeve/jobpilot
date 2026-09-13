# Feature: Onboarding

## Purpose

Prepare a local JobPilot installation for use.

## Requirements

- Detect supported OS.
- Initialize local data directory.
- Initialize SQLite database.
- Configure initial LLM provider.
- Create candidate profile.
- Import optional resume.
- Configure job-search preferences.

## Acceptance Criteria

### AC-001
Given a new installation, when onboarding starts, then required local storage is initialized.

### AC-002
Given Ollama is selected, when connectivity is tested, then the user receives a clear success/failure result.

### AC-003
Given no internet connection, when onboarding only configures local data, then the user can complete local setup.

### AC-004
The application must not require creating an account.

### AC-005
Onboarding ends with at least one saved search configured.

Discovery refuses to run on empty criteria, so an installation without a
saved search cannot find jobs at all. The step is therefore not skippable
the way resume import is: the CLI's `onboard` re-prompts until it gets a
named search with at least one criterion.

## Implementation status

`SetupEnvironment`, `ConfigureLLM`, `InitializeProfile`, `ProcessResume` and
`ConfigureSearchPreferences` cover every requirement above, driven by
`jobpilot-cli onboard`.

The desktop app has no onboarding wizard. Its equivalents are spread across
screens — the Profile screen creates a candidate, Settings configures the LLM
provider and job searches — and the Jobs screen points at Settings when no
search exists.
