# Feature: Onboarding

## Purpose

Prepare a local Job Agent installation for use.

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
