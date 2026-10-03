# ADR-0027: When a Scout runs on its own

## Status

Accepted on 2026-10-02, and implemented.
The four choices below were made that day: the operating system's scheduler starts a Scout, a schedule is every N hours or daily at a time, Scout runs happen one at a time, and a missed run runs once.
Decisions 1 to 9 say how they are built, and were accepted with them.

It settles the trigger ADR-0014 and ADR-0018 deferred, within ADR-0009.
It changes nothing about what a run does: a scheduled run is the run that exists today, started by a clock instead of a click.

## Context

ADR-0014 deferred how a Scout is triggered: "there is no 24/7 daemon, so a schedule is either the desktop application starting work when it is open, or an OS-level scheduler starting the daemon".
Building it turns up five things that bear on how.

### The daemon outlives the desktop, but not a restart

The desktop and the CLI both start the daemon as a detached process when none answers (`desktop/src-tauri/src/daemon.rs`, `cmd/jobpilot-cli/daemon.go`).
Neither stops it, and idle exit is not built (`docs/01_architecture/01_daemon.md`), so once started it runs until the user logs out or the machine restarts.
After a restart nothing is running until the user opens JobPilot.

### Two runs can create the same application twice

A run reads the jobs the candidate already has applications for once, at its start (`jobsAlreadyApplied`), and nothing else stops a duplicate.
There is no lock around a run and no unique constraint on `applications (candidate_id, job_id)`.
Two runs that overlap, of one Scout or of two whose searches share a job, can each create an application for it.
Today that takes a user starting runs from the CLI and the desktop at once; a schedule makes it routine.

### A run belongs to the request that started it

`POST /v1/scouts/{id}/run` runs the Scout inside the request and passes the request's context.
A run started by the operating system must not depend on how long the operating system lets the process that started it live.

### The only binary JobPilot can count on is the daemon

The desktop finds the daemon next to its own executable, then on `PATH` (`find_daemon_binary`).
The CLI is not bundled with the desktop, so an OS entry cannot point at `jobpilot-cli`.
The daemon knows where it is (`os.Executable`), so it can write entries that point at itself.

### Each operating system schedules differently

- macOS: a `launchd` agent in `~/Library/LaunchAgents`, loaded with `launchctl bootstrap gui/<uid>`. Since Ventura, adding one raises a "Background Items Added" notification, and the user can switch it off in System Settings → Login Items without JobPilot being told.
- Linux: a `systemd --user` service and timer in `~/.config/systemd/user`. A user service does not inherit the shell's environment, and `os.UserConfigDir` follows `XDG_CONFIG_HOME`, so a daemon started by systemd can look for its data somewhere other than where the user's does. A system without systemd has no standard user scheduler.
- Windows: a Task Scheduler task, created with `schtasks /Create /XML`. By default a task does not start on battery power and does not run a start time that was missed.

Each entry must follow its Scout: rewritten when the schedule changes, removed when the Scout is deleted, disabled or unscheduled, and removed when JobPilot is uninstalled, which nothing in JobPilot observes.
An entry left behind fires at every scheduled time, forever, and fails.

The keystore (ADR-0019) is the measured warning about platform integrations: only macOS was verified, and what macOS did was hang rather than fail.
No CI runs on Linux or Windows.

## Options

**A. The daemon keeps the time.** A scheduler inside the daemon runs what is due, while the daemon is running. No OS integration. Nothing runs after a restart until JobPilot is opened.

**B. The operating system keeps the time.** Each scheduled Scout gets an entry in `launchd`, `systemd` or Task Scheduler, which starts JobPilot to run it. It runs after a restart without the user opening anything, provided they are logged in and the machine is awake. It costs three integrations kept in step with every schedule edit.

**C. A, and the daemon starts at login.** One OS registration that never changes, closing A's gap after a restart.

## Decision

**B, chosen on 2026-10-02.**
A Scout runs on schedule whether or not JobPilot has been opened since the machine started.

### 1. A schedule is typed, not a cron expression

```
Scout
  schedule          "off" | "every" | "daily"
  every_hours       1..24, for "every"
  daily_at          minutes after local midnight, for "daily"
```

"Off" is the default, and every existing Scout gets it.
Two shapes cover what a job search needs: check often, or check each morning.
A cron string can express anything and be checked for nothing, which is the `policies.rule` column ADR-0018 dropped.
Days of the week are left out until someone asks.

`daily_at` is in the computer's local time, not the profile's time zone: the profile says where the candidate works, the clock on the machine says when they are at it.

### 2. The OS is the alarm, and core decides whether a Scout is due

An OS entry fires and asks; it does not decide.
When it fires, core runs the Scout only if it is still enabled, still scheduled, and due:

- **daily at T**: no run of it has started since the most recent T.
- **every N hours**: no run of it has started in the last N hours.

Both allow five minutes of grace, so an alarm a few seconds early is not taken as a time not yet reached.

Due is measured from the later of two times: when the last run started, and when the schedule was set.
From the start, so "every 6 hours" does not drift by the length of each run; a manual run counts.
From when it was set, so a Scout scheduled at 10:00 for "daily at 08:00" first runs tomorrow at 08:00, not the moment it is saved.
`scouts.schedule_set_at` records it, and changes only when the schedule does: renaming a Scout does not move its next run.
A failed run counts too: a model that is not running fails a run at once, and the next scheduled time is the retry.

This keeps the rule in core, where it is tested against a fake clock, and makes an alarm that fires twice, early, or for a Scout that changed since its entry was written, do nothing rather than something wrong.

The alarm is set to:

- **daily at T**: T each day.
- **every N hours**: each hour, on the hour. Core runs the Scout on the first hour at which it is due, so it runs within the hour of when it is due.

An alarm for "every N hours" set to fire every N hours from when it was written would, after a manual run, find the Scout not due and wait another N hours. Hourly avoids that at the price of 24 checks a day, each an HTTP request to a running daemon.

### 3. A missed run runs once

A machine asleep or logged out through three daily runs runs once, not three times.
Each platform is set to run a missed alarm once at the next opportunity: `systemd` timers use `Persistent=true`, and Task Scheduler tasks use `StartWhenAvailable`, both of which also cover a machine that was off.
`launchd` coalesces missed calendar intervals into one on wake, but not across a shutdown, so the macOS entry also fires when it is loaded, at login, with `RunAtLoad`.
Core's check (decision 2) makes a second catch-up harmless if a platform delivers one.

### 4. The alarm wakes the daemon, and the daemon runs the Scout

An entry runs the daemon's own executable with one subcommand:

```
jobpilot scout wake <scout-id>
```

`scout wake` is a client, not a second daemon.
It finds the running daemon through the discovery file, starts one as the CLI does if none answers, sends `POST /v1/scouts/{id}/wake`, and exits.
The single-daemon rule (`refuseSecondDaemon`) holds without exception.
The daemon-finding code moves from `cmd/jobpilot-cli` to a package both binaries use.

The daemon answers at once: started, not due, disabled, or unknown.
The run itself goes on under a context the daemon owns, which shutdown cancels and records as failed (AC-006).
So the operating system never holds a process open for the length of a run, and no platform's limit on how long a task may take applies to one.

A daemon started by an alarm stays running, as one started by the desktop does, until idle exit is built (ADR-0009 allows it to start "for scheduled work").

### 5. Scout runs happen one at a time, whatever started them

The daemon holds one lock for all Scout runs, manual and scheduled.
A run that finds another in progress waits for it.

That stops the duplicate application in every case, including two Scouts whose searches share a job, because the second run reads existing applications after the first has created them.
It also keeps two runs from scoring against one local model at once.
A manual run that waits says it is waiting, and for which Scout.

A unique constraint on applications per job would also stop duplicates, but it would decide that a job can never be applied to twice, which is a product question nobody has asked.

### 6. The database is the schedule, and the entries are reconciled to it

The `scouts` table is the only record of what is scheduled.
The OS entries are derived from it and are never read back as a source of truth.

A port in core, `ports.ScoutAlarms`, has one adapter per platform and three operations: list JobPilot's entries, write one, remove one.
Reconciling compares the entries the enabled, scheduled Scouts need with the entries that exist, then writes the missing or different ones and removes the rest.
It runs:

- after every change to a Scout (create, update, enable, disable, delete), and after a saved search's deletion cascades to its Scouts;
- when the daemon starts, which repairs an entry the user removed and rewrites entries whose executable path has changed, because JobPilot was moved or rebuilt;
- before data deletion (ADR-0021) removes Scouts.

A Scout's change is saved even when its entry cannot be written.
The Scout then shows that it is not scheduled, and why, until a later reconcile succeeds.
A save that failed because `launchctl` did is worse than a saved Scout that says it is not yet scheduled.

An entry is JobPilot's by its name: one prefix per platform followed by the Scout's ID.
A Scout ID is checked to be a UUID before it is written into a plist, a unit file or task XML, and every value goes through that format's encoder, not string concatenation.

### 7. Per platform

| | Entry | Alarm | Missed alarm | Notes |
| --- | --- | --- | --- | --- |
| macOS | `~/Library/LaunchAgents/<prefix>.scout.<id>.plist`, loaded with `launchctl bootstrap` | `StartCalendarInterval`, and `RunAtLoad` | coalesced on wake; checked at login | The user can disable it in Login Items; reconcile reports that, and does not re-enable it behind their back. |
| Linux | `jobpilot-scout-<id>.service` (`Type=oneshot`) and `.timer` in `~/.config/systemd/user`, enabled with `systemctl --user` | `OnCalendar` | `Persistent=true` | The unit sets `XDG_CONFIG_HOME` when the registering daemon had one. Without systemd, scheduling is unavailable and says so. |
| Windows | Task `\JobPilot\scout-<id>`, created from XML | `CalendarTrigger`, daily or hourly | `StartWhenAvailable` | Runs on battery, runs as the logged-in user with no stored password, and starts the daemon without a console window. |

The prefix for macOS is the app's bundle identifier, which today is the placeholder `com.user.jobpilot`.
It should be settled before release: changing it later means reconcile has to recognise both.

### 8. Paused and unscheduled are different things

`is_enabled` keeps its meaning: a disabled Scout does not run, by hand or by schedule, and has no entry.
`schedule = off` means only that nothing starts it but the user.
Pausing a Scout for a holiday does not lose its schedule.

### 9. What the user sees

A scheduled Scout shows its next run, its last run and that run's outcome, and whether the operating system has its entry.
`agent_runs` gains `trigger`, `manual` or `schedule`, so the activity screen can say which runs happened on their own: "what did it do while I was away" is the first question a schedule raises.

The schedule setting says what the operating system will do, in its terms: macOS will report a background item from JobPilot, and a Scout runs while the user is logged in and the machine is awake.

## Consequences

- A migration adds `schedule`, `every_hours`, `daily_at` and `schedule_set_at` to `scouts`, and `started_by` to `agent_runs`, which the API calls `trigger`. Existing Scouts are unscheduled, and existing runs are `manual`, which they all were.
- `ScoutService.Run` takes the lock, so a manual run can wait behind a scheduled one. Today it never waits.
- The daemon binary gains its first subcommand, `scout wake`.
- JobPilot writes outside its data directory for the first time: into the user's LaunchAgents, systemd user units, or Task Scheduler.
- An uninstalled JobPilot leaves its entries behind, firing and failing. Until packaging exists (`docs/00_product/01_requirements.md`, post-MVP), a user removes them by deleting their Scouts, turning schedules off, or deleting their data. Packaging, when it is built, must remove them on uninstall.
- macOS is measured before this is called done. Linux and Windows are built to the same port and tested on the files they generate, and listed in `current_state.md` as unverified, as the keystore is.
- A development daemon (`make dev`, in `bin/dev/`) rewrites the entries to point at itself when it starts. That is correct for the machine it runs on, and worth knowing.
- `docs/02_features/10_scouts.md` loses "Deferred: how a Scout is triggered" and gains acceptance criteria for the schedule.
- `docs/01_architecture/01_daemon.md`'s lifecycle pattern 3, "OS scheduler starts the daemon", stops being aspirational.
- Notifications stay post-MVP (`docs/02_features/11_notifications.md`). A scheduled run's applications appear in the list, and its failure in the activity screen and on the Scout, with nothing pushed.

## What building it changed

Building it, and measuring it on macOS 27 with a real launchd agent, settled several details the decision left open.

- **A daily time is read in the computer's time zone.** Times read back from SQLite are in UTC, and the first version worked a daily time out in the zone of the time it measured from, so "daily at 08:00" fell at 09:00 in Lagos. The desktop showed it; `Due` and `NextRun` now put that time in the zone of `now`, the computer's.
- **The daemon an alarm starts must outlive the alarm.** launchd kills what a finished job leaves in its process group, and systemd what a one-shot service leaves in its cgroup. The agent sets `AbandonProcessGroup`, the unit `KillMode=process`, and the daemon is started in a session of its own (`Setsid`), or with no console on Windows, so it does not depend on either. Measured: with no daemon running, an alarm started one, which kept running after the alarm's job ended, and the run was recorded.
- **The macOS agent is `ProcessType Interactive`.** The daemon an alarm starts is the one the desktop then talks to, and launchd throttles a background job's children as it throttles the job.
- **On Windows the task starts the daemon through `conhost.exe --headless`**, so no console window opens on every alarm, and `schtasks` itself runs with no window, since the daemon that runs it has no console.
- **Reconciling at start-up happens in the background**, so a slow scheduler does not hold up the daemon becoming reachable to the desktop, which waits only seconds for it.
- **The run's trigger is stored as `agent_runs.started_by`**, since `trigger` is a keyword in SQLite.
- **Three existing faults had to go first.** Editing a scout erased its last run, because the update wrote `last_run_at` from a form that does not carry it; a finishing run rewrote the whole scout row, which could undo an edit made during the run; and the CLI's client cut off every request at 60 seconds, which cancelled a long `scout run` in the daemon and would have cut short a letter from a slow local model. Due is measured from runs, so the first two would have moved schedules. Updating a scout no longer writes its last run, a run records only its time (`ScoutRepository.MarkRun`), and the client bounds connecting rather than the whole request.

Measured on macOS: the agent loads with its environment, runs at load, an alarm the daemon finds not due starts nothing, a due one starts a run recorded as `schedule`, a cold start works as above, a changed time rewrites the plist, and deleting the scout unloads the agent and removes its file. `launchctl print-disabled` prints `=> enabled` and `=> disabled` on macOS 27, as the adapter reads it.
Linux and Windows are built to the same port and tested on the units and task XML they generate, and not yet run on either system.

## Alternatives considered

**A, the daemon keeps the time.** No OS integration and one implementation of "due", but nothing runs after a restart until JobPilot is opened.

**C, A plus starting at login.** One registration that never changes, rather than one per Scout kept in step. Not chosen.

**One OS entry for all Scouts, firing hourly.** Core would pick which are due. It needs no rewriting when a schedule changes, but a daily Scout would run up to an hour late, and the operating system's own view of what JobPilot schedules would say nothing about which Scout or when.

**The OS entry runs the Scout itself, in its own process.** It would open the database beside a running daemon, which the single-daemon rule exists to prevent, and would hold an OS task open for as long as a run takes.

**The desktop keeps the time.** It stops when the window closes, though the daemon is still running, and puts "is it due" in TypeScript beside a Go core that owns every other rule.

**A cron expression.** Above, under decision 1.

**Run every missed occurrence.** Three runs after three days asleep would read the same jobs three times and create nothing the first did not.

**Retry a failed run sooner.** A run fails mostly because the model or the network is not there, and neither comes back on a schedule JobPilot can guess.
