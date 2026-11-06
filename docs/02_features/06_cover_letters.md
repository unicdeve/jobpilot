# Feature: Cover Letters

## Requirements

The candidate writes their own cover letter for an application, in JobPilot, beside the job and their own material ([ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md)).
JobPilot never writes, drafts or rewrites a letter.

What the candidate has said about themselves, under About you, is shown beside the application ready to copy (`16_about_you.md`).
Draft checks point out stock phrases, an unusually long letter and claims the candidate's own material does not support (`19_draft_checks.md`).

## Rules

- No model writes any part of a letter.
- A letter is kept as versions: saving a new one keeps the earlier ones, and the current version can be edited in place.
- An empty letter is refused.
- Draft checks advise; they never block saving.

## Acceptance Criteria

### AC-001
A user can write and save a cover letter for an application.

### AC-002
Saving a new letter adds a version and makes it the application's current letter.

### AC-003
No code path generates letter text.

### AC-004
The user can edit the current version in place, and copy it.

### AC-005
Every version is stored in application history until the user deletes an earlier one.
