# ADR-0028: Telling the user what a scout found

## Status

Accepted on 2026-10-03, and implemented.
Four choices were made that day: the daemon asks the desktop app to post a notification, scout runs notify (scheduled runs always, any run when it finds a strong match), a notification says counts and a scout's name and nothing else, and email comes later on the same design.
Decisions 1 to 8 say how they are built, and were accepted with them.

It builds the desktop half of `docs/02_features/11_notifications.md`, which was post-MVP, and the `Notification` entity `docs/01_architecture/04_database.md` lists and nothing stores.

## Context

ADR-0027 made scouts run on their own, and left them silent: "a scheduled run's applications appear in the list, and its failure in the activity screen and on the Scout, with nothing pushed."
A scout that runs at 08:00 while the user is elsewhere is only useful if they find out, and today they find out by opening JobPilot.

Four things bear on how.

### The daemon is the only part that is there

A scheduled run happens in the daemon, often started by the run's own alarm.
The desktop app is usually not running: closing its window quits it, and it has no tray icon.
So whatever decides that something is worth telling has to be the daemon.

### A notification belongs to an app

macOS attributes a notification to an application bundle, shows its name and icon, and opens it when the notification is clicked.
A plain executable has no bundle: the common workaround, `osascript -e 'display notification'`, posts as Script Editor, and clicking it opens Script Editor.
Windows ties a toast to an application user model ID, which an installed app registers and a loose executable does not have.
Linux asks a D-Bus notification server, which shows whatever name it is given.

The desktop app has the right identity on all three: Tauri's notification plugin posts under the app's bundle identifier or app ID, and clicking a notification opens the app.

### Notifications show on a lock screen

What a notification says is visible to anyone who can see the screen, without unlocking it.

### Email is in the spec, and is a different kind of thing

`11_notifications.md` lists email, disabled unless configured.
It needs an SMTP server and a password in the keychain (ADR-0019), it sends data off the machine, and it fails in ways a desktop notification does not.

## Decision

### 1. The daemon decides, records, and asks the app to post

When something is worth telling, the daemon records a notification, then starts the desktop app in a delivery mode:

```
<desktop executable> --deliver-notifications
```

In that mode the app opens no window, shows no Dock icon (an accessory app on macOS), asks the daemon for the notifications not yet posted, posts each through the notification plugin, tells the daemon each was posted, and exits.
If the app is already open, a second, short-lived process posts them, under the same name, and clicking one brings the open app forward.

A notification is claimed before it is posted, `POST /v1/notifications/{id}/posted` answering only the first caller, so two deliveries never post one notification twice.

The daemon learns where the desktop app is from the app itself: on every start the app tells the daemon its executable's path, and the daemon keeps the last one in its settings.
That works for an installed app and for `tauri dev` alike, and needs no guess about where the two binaries are installed relative to each other.
A daemon that has never been told, or whose path no longer exists, records the notification and posts nothing.

### 2. What is worth telling

A scout run tells the user in two cases.

- **A scheduled run** that created applications, or failed.
  A scheduled run that found nothing says nothing.
- **Any run**, scheduled or started by hand, that created an application for a **strong match**.
  A run started by hand can score dozens of jobs with a local model and take many minutes, so the user is often elsewhere when it ends.

A run tells once, however many things it found: a scheduled run with strong matches says both in one notification.

Analysing one job does not notify. The user started it and is looking at the job when it finishes.
Nothing else notifies yet.

A strong match is an application created at or above a score the user sets, 80 unless they change it.
It is a threshold in core, compared with arithmetic, like a scout's floor (AGENTS.md principle 7).

### 3. A notification says counts and a scout's name

```
Morning Go roles found 3 jobs for you
2 of them are strong matches. They're ready for review.

Morning Go roles couldn't run
Open JobPilot to see why.
```

No job title, company, score, salary or reason for a failure: none of what the user is applying for, and nothing from a model's error message, reaches a lock screen.
The scout's name is the user's own label.
What the run found is one click away.

The text is written in core, so the CLI's list and the desktop's say the same thing.

### 4. Every notification is kept, and shown in the app

```
notifications
  id, kind, title, body
  scout_id        -> scouts, ON DELETE SET NULL
  run_id          -> agent_runs, ON DELETE SET NULL
  created_at
  posted_at       when the operating system was given it
  seen_at         when the user saw it in JobPilot
```

The desktop shows the notifications the user has not seen, on the Dashboard, and marks them seen when they are dismissed.
So one that could not be posted — the app's path unknown, permission refused, no notification server — is not lost, and nothing depends on the operating system's own list, which the user can clear.

A notification older than an hour that was never posted is not posted late. A toast about this morning, shown at dinner, misleads more than it tells; it waits in the app instead.

### 5. One column per channel

`posted_at` is the desktop's.
Email, when it comes, adds `emailed_at` and its own settings, and delivers from the same records.
That is all the design email needs now.

### 6. Settings

Settings → Notifications holds two things, in the daemon's `user_settings` so the CLI shares them:

- whether to post notifications at all, on by default;
- the score a strong match needs, 80 by default.

Turning notifications off stops posting, not recording: the app still shows what the user has not seen.
The operating system's own permission is the other switch, and macOS asks for it the first time JobPilot posts.

### 7. Delivery never holds up a run

Recording a notification is part of finishing a run, best-effort like the rest of its trail: a failure is logged and the run's result stands.
Starting the desktop app happens after, in the background, bounded, and its failure is logged.

### 8. The CLI reads the same records

`jobpilot-cli notifications` lists the ones not seen, and marks them seen; `--all` lists the rest.

## Consequences

- A migration adds `notifications`. The entity `04_database.md` listed finally has a table.
- The desktop app gains a second way to start, which must open no window. Its main window moves from the configuration file to being created at start-up, unless the app is delivering notifications.
- The desktop gains `tauri-plugin-notification`, and the daemon two endpoints for it: `GET /v1/notifications` and `POST /v1/notifications/{id}/posted`, with `/seen` beside them.
- A notification posted by `tauri dev` is attributed to the terminal on macOS, since an unbundled app has no bundle. Only a built app shows as JobPilot.
- Windows shows a toast as JobPilot only once the app is installed with its app ID; until there is an installer it is attributed to PowerShell, as Tauri's plugin does in development.
- Clicking a notification opens the app, not the applications it names: Tauri's plugin reports no click on the desktop. The Dashboard, where the app opens, lists what is unseen first.
- `11_notifications.md` is rewritten around this: the desktop channel built, email designed for and deferred.

## What building it changed

- **A run is recorded before anything that can fail.** The first real delivery posted two notifications for one scheduled run: with no profile, the run failed before it recorded itself, so the scout stayed due, and the alarm's run-at-load and the next wake each ran it. A failure to load the profile or the saved search now ends a recorded, failed run, which shows in Activity and makes the scout not due until its next time, as ADR-0027 §2 meant.
- **The main window is created at start-up**, with `"create": false` on it in both Tauri configurations, so delivery mode opens none.
- **The daemon waits for scout runs, then for deliveries, on shutdown**, since a run that finishes can still record a notification and start one.

Measured on macOS with the debug desktop binary: an alarm's failing run recorded one notification, the daemon started the app in delivery mode, the app claimed and posted it and exited, leaving no process and no window.
The Dashboard's list, dismissing, Settings → Notifications and the CLI were checked against a daemon of their own.
Not yet seen: a notification posted by a built, bundled app, which is the one shown as JobPilot.

## Alternatives considered

**The daemon posts it itself**, with `osascript`, `notify-send` and PowerShell. No desktop involvement, but on macOS every notification is Script Editor's and opens Script Editor, and Windows has no app ID to attribute it to.

**Only while the app is open**, with a summary when it next opens. Simplest, and silent in the one case this is for: the app closed, the scout running at 08:00.

**A native notifier in the daemon**, through cgo on macOS and COM on Windows. Right identity only if the daemon runs from inside the app bundle, and a second implementation of what Tauri already does, in a codebase that has kept cgo out.

**Job titles in the notification.** More useful at a glance, and shows what the user is applying for to anyone near the screen.

**Email now.** Its own settings, secret, failure handling and privacy question, for a channel nobody has asked to use yet.
