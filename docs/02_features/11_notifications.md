# Feature: Notifications

## Status

Desktop notifications are implemented (ADR-0028).
Email is designed for and not built.

## Purpose

A scout that runs at 08:00 while the user is elsewhere is only useful if they find out.
A notification tells them what it found, or that it could not run, whether or not JobPilot is open.

## What notifies

A scout run tells the user in two cases:

- **A scheduled run** that created applications, or failed. One that found nothing says nothing.
- **Any run**, scheduled or started by hand, that created an application for a **strong match**: one whose match scores at or above the user's threshold, 80 unless they change it.

A run tells once, however many things it found.
Analysing a single job does not notify: the user is looking at it when it finishes.
Nothing else notifies yet.

## What a notification says

Counts and a scout's name, and nothing else, because a notification shows on a lock screen:

```
Morning Go roles found 3 jobs for you
2 of them are strong matches. They're ready for review.

Morning Go roles couldn't run
Open JobPilot to see why.
```

No job title, company, score, salary, or the reason a run failed.
What the run found is in JobPilot, one click away.

## How it reaches the screen

The daemon decides what is worth telling and records it.
It then starts the desktop app in a delivery mode, which opens no window, shows no Dock icon, posts the notification under JobPilot's own name, and exits.
The daemon knows where the app is because the app tells it each time it starts.

Every notification is kept.
The Dashboard lists those the user has not seen, under "While you were away", with each linking to the run it is about; dismissing one marks it seen.
So a notification the operating system never showed — the app's location unknown, permission refused, notifications turned off — still reaches the user.
One more than an hour old that was never posted is not posted late; it waits on the Dashboard.

## Settings

Settings → Notifications, shared with the CLI (`jobpilot-cli settings notifications`):

- whether to post notifications about scout runs, on by default; turned off, they still wait on the Dashboard;
- the score a strong match needs, 1 to 100, 80 by default.

The operating system's own permission is the other switch. macOS asks for it the first time JobPilot posts.

## Email

The spec has always listed email, disabled unless configured.
It is not built: it needs an SMTP server, a password in the keychain (ADR-0019), its own failure handling, and it sends data off the machine.
Each notification is kept with a column per channel, so email adds `emailed_at` and its settings and delivers from the same records (ADR-0028 §5).

## Acceptance Criteria

### AC-001
A scheduled scout run that created applications or failed posts one notification, whether or not the desktop app is open.

### AC-002
A run started by hand posts a notification only when it created an application for a strong match.

### AC-003
A notification names no job, company, score or error; only counts and the scout's name.

### AC-004
Notifications are not posted when turned off, and are still shown in the app.

### AC-005
A notification is posted at most once, even when the app is open as one is delivered.

### AC-006
A notification that could not be posted is shown on the Dashboard until the user dismisses it.

### AC-007
Failed delivery does not change what the run did or recorded.

## Out of scope

- Email, until someone asks to use it.
- Opening the applications a notification names when it is clicked. Tauri's notification plugin reports no click on the desktop, so a click opens JobPilot, which opens on the Dashboard.
- Notifying about anything but scout runs.
