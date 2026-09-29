# Feature: Cover Letters

## Requirements

Generate a tailored cover letter from:
- job description
- candidate profile
- relevant resume content
- user prompt preferences

What the candidate says about themselves — the About you answers, this application's answers, the writing sample and how they want to sound — is `docs/02_features/16_about_you.md`, which also sets the default prompt's style rules.

## Rules

- Never invent experience.
- Avoid generic filler where relevant evidence exists.
- Respect user-selected tone and length.
- Generated content is editable.

## Acceptance Criteria

### AC-001
A user can generate a cover letter for a stored job.

### AC-002
The letter references relevant candidate experience when available.

### AC-003
The system does not invent candidate facts.

### AC-004
The user can regenerate and edit the result.

### AC-005
The generated content is stored in application history.
