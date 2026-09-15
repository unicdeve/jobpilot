# Feature: Application Management

## State model

A job and an application are separate entities.

Application states:

DISCOVERED
→ MATCHED
→ PREPARING
→ READY_FOR_REVIEW
→ APPROVED
→ SUBMITTED
→ INTERVIEWING
→ OFFERED
→ NO_RESPONSE
→ REJECTED
→ WITHDRAWN
→ FAILED

Not every job must progress through every state.

### Before submission

The states up to and including SUBMITTED are what the application does for the user: find, match, prepare, review, approve, send.
SUBMITTED is reachable only from APPROVED, and only through an explicit user action.

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

WITHDRAWN becomes reachable from SUBMITTED, INTERVIEWING and OFFERED. It is
currently not reachable from any of them, which is wrong in an obvious way: a person can withdraw an application they have sent, decline an interview, and turn down an offer, and today the model offers them no way to say so.

## Acceptance Criteria

### AC-001
Application state transitions are validated.

### AC-002
Invalid transitions are rejected.

### AC-003
Application history is persisted.

### AC-004
An application can reference generated answers and cover letters.

### AC-005
A job cannot silently become marked as submitted.

### AC-006
Post-submission outcomes are recorded through the same validated transitions, and appear in the application's history like any other.

### AC-007
No outcome is inferred. Every state after SUBMITTED is entered by an explicit user action, never by elapsed time or by any other event.
