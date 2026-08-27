# Feature: Autopilot

## Status

Post-MVP.

## Requirements

Autopilot may perform external application submission only when:
- mode is Autopilot
- policy rules permit the application
- required data is known
- no blocking condition exists

## Safety

- CAPTCHA stops automation.
- Unknown required factual questions stop automation.
- Policy denial stops automation.
- Browser/session failure stops automation.
- Ambiguous fields stop automation.

## Acceptance Criteria

### AC-001
Autopilot cannot submit when policy denies the action.

### AC-002
Autopilot cannot fabricate candidate facts.

### AC-003
Blocked applications are persisted with a reason.

### AC-004
Successful submission produces a persistent result.
