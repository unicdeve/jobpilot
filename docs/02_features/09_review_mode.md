# Feature: Review Mode

## Purpose

Provide user control before consequential application submission.

## Requirements

- Review is the default mode.
- Show generated answers.
- Show match analysis.
- Show job details.
- Show proposed external actions.
- Require explicit approval before submission.

## Acceptance Criteria

### AC-001
New installations default to Review mode.

### AC-002
The user can inspect generated content before submission.

### AC-003
Submission cannot occur from Review mode without explicit approval.

### AC-004
Approval is recorded with the application action history.
