# Feature: Review Mode

## Purpose

Let the user inspect generated content and a prepared browser form before they submit it themselves.
JobPilot never clicks an external application's submit control ([ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md)).

## Requirements

- Review is the default mode.
- Show generated answers.
- Show match analysis.
- Show job details.
- Show proposed external actions.
- Record explicit approval of the application review.
- Leave the prepared browser window open for the user.
- Record submission only when the user says they submitted it externally.

## Acceptance Criteria

### AC-001
New installations default to Review mode.

### AC-002
The user can inspect generated content before submission.

### AC-003
JobPilot never submits the application, including after approval.
The user submits it in the browser.

### AC-004
Approval is recorded with the application action history.
