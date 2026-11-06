# Feature: Application Management

## State model

A job and an application are separate entities.

The path to sending is DISCOVERED → MATCHED → APPROVED (ready to send) → SUBMITTED.
APPROVED is optional: the user can record SUBMITTED straight from MATCHED, and move a ready-to-send application back to MATCHED.
Outcomes branch from SUBMITTED; they are not a single sequence.
PREPARING, READY_FOR_REVIEW and FAILED existed for browser preparation and were removed with it ([ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md) §3).
The transition table in `internal/core/services/application.go` is authoritative, and the API exposes allowed next states to clients.

### Before submission

JobPilot finds and matches jobs, and keeps what the candidate needs to apply beside the job:

- "Open application page" opens the job's link, or one the user saved for the form itself, in the user's own browser.
- Their profile details, About you answers and kept answers are listed, each a click from the clipboard.
- Their cover letter (`06_cover_letters.md`) and their notes on the form's questions (`07_application_answers.md`).

It never writes the application, fills in the form, or sends it.
SUBMITTED is reachable only from MATCHED and APPROVED, when the user explicitly records that they submitted externally.

### After submission

The states after SUBMITTED are what happens to the user, and they are the part of a job search that lasts.
They exist so an application that was sent does not become invisible:

- INTERVIEWING — the employer replied and a process is under way.
- OFFERED — an offer was made.
- NO_RESPONSE — the user closes it out having heard nothing back.

None of these is ever inferred.
NO_RESPONSE in particular is a decision the user makes, not a timer: the application cannot know whether silence means rejection, a slow process, or an email that went to spam, and guessing would put a fact in the record that nobody established.

A late reply reopens a closed-out application: NO_RESPONSE can move to INTERVIEWING or REJECTED.

### Withdrawing after submission

WITHDRAWN is reachable from SUBMITTED, INTERVIEWING and OFFERED.
Recording it does not contact the employer; the user handles any external withdrawal or offer response themselves.

## Acceptance Criteria

### AC-001
Application state transitions are validated.

### AC-002
Invalid transitions are rejected.

### AC-003
Application history is persisted.

### AC-004
An application can reference the candidate's own answers and cover letters.

### AC-005
A job cannot silently become marked as submitted.

### AC-006
Post-submission outcomes are recorded through the same validated transitions, and appear in the application's history like any other.

### AC-007
No outcome is inferred.
Every state after SUBMITTED is entered by an explicit user action, never by elapsed time or by any other event.
