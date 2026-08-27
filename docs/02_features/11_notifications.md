# Feature: Notifications

## Status

Post-MVP.

## Channels

- Email
- Desktop notification

## Email

Configurable by the user and disabled unless configured.

## Notification events

- strong job match
- application ready for review
- application submitted
- application failed
- scheduled search failed

## Acceptance Criteria

### AC-001
Users can enable/disable email notifications.

### AC-002
Notifications are not sent when disabled.

### AC-003
Failed delivery does not corrupt application state.
