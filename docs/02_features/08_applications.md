# Feature: Application Management

## State model

A job and an application are separate entities.

Suggested application states:

DISCOVERED
→ MATCHED
→ PREPARING
→ READY_FOR_REVIEW
→ APPROVED
→ SUBMITTED
→ REJECTED
→ WITHDRAWN
→ FAILED

Not every job must progress through every state.

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
