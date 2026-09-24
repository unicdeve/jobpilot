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

The desktop wizard offers the search templates first and a blank form second
(`docs/02_features/14_search_templates.md`). This is the point in the product
where a user is least able to tell a search that will work from one that will
quietly find nothing, because nothing has been discovered yet to judge it by.

## Implementation status

`SetupEnvironment`, `ConfigureLLM`, `InitializeProfile`, `ProcessResume` and
`ConfigureSearchPreferences` cover every requirement above, driven by
`jobpilot-cli onboard`.

The desktop app covers the same ground with a first-run wizard at
`/welcome` (`desktop/src/pages/welcome.tsx`), in the CLI's order and for the
CLI's reason: prefilling a profile from a resume needs the LLM, so the
provider comes first. An installation with no candidate profile is sent
there from wherever it lands; skipping is remembered for the session, and a
banner offers the way back for anything still missing.

Which step opens is derived from what exists — saved LLM settings, a
candidate, a resume, a saved search — rather than from a stored "completed"
flag, so the wizard resumes at the first gap and cannot disagree with the
database about what is done.

`SetupEnvironment` has no desktop equivalent because it needs none: the
daemon initializes the data directory and runs migrations at startup, so
AC-001 is satisfied before the app can connect at all.
