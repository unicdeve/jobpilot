# Feature: Scouts

## Status

Implemented. Roadmap Phase 8.

The data model is ADR-0018. A scout is a saved search plus a match-score floor and a per-run ceiling: the other rules below live on the saved search, which already expresses and applies them.

A scout runs when the user asks, or on a schedule kept by the operating system's scheduler (ADR-0027).

Replaces the Autopilot feature, per
`docs/09_decisions/0014-scouts-and-no-unattended-submission.md`.

## What a Scout is

A standing instruction the user configures once, separate from any single
application.

A Scout runs a saved search, analyzes what it finds against the candidate,
and creates applications for the ones worth the user's attention.
The output is a shortlist waiting for them.
The user opens one, reviews the match, and prepares it in the browser
themselves — the flow that already exists, entered from a list someone else
assembled.

Autopilot, the feature this replaces, fused two separate ideas: unattended
work, and automatic submission.
They arrived in one sentence and were treated as one feature.
Scouts keep the first.
The second was declined — see "Why nothing submits" below.

## A run

1. Resolve the Scout's saved search and its eligibility rules.
2. Obtain jobs to consider: those already stored locally, or more fetched
   from the configured sources, as the Scout is set up.
3. Analyze each against the candidate, through the existing match service.
4. Create an application, in the ordinary initial state, for each job that
   clears the rules.
5. Record the run and every decision in it in the agent trail.
   A fetching Scout records each source that could not be reached or
   answered with an error as its own step, so a run that considered fewer
   jobs than usual says why. A run no source answered fails.

## Boundaries

- A Scout contacts no employer. It stops before the browser.
- A Scout's only external traffic is the job-source APIs discovery already
  calls.
- A Scout creates applications. It never submits one, and neither does
  anything else in JobPilot.
- Eligibility rules are deterministic and live in core. An LLM never decides
  whether a job clears them.
- A Scout cannot invent candidate facts, because it writes no answers.

## Schedule

A scout runs only when the user runs it, every N hours (1 to 24), or once a day at a time on the computer's clock.
There is no cron expression: two shapes cover a job search, check often or check each morning, and each is two typed columns.

The operating system's scheduler starts it: launchd on macOS, a systemd user timer on Linux, Task Scheduler on Windows.
So a scheduled scout runs whether or not JobPilot has been opened since the machine started, while the user is logged in and the machine is awake.

- The alarm only asks. It runs `jobpilot scout wake <id>`, which asks the daemon, starting it if none is running, to run the scout if it is due, and exits.
  Whether the scout is due is decided in core, so an alarm that fires twice, early, or after the scout changed does nothing wrong.
- Due is measured from the later of the last run's start and when the schedule was set.
  A manual run counts. A failed run counts too, and the next scheduled time is its retry.
  A scout scheduled at 10:00 for "daily at 08:00" first runs tomorrow.
- "Every N hours" is checked hourly, on the hour, so it runs within the hour of being due.
- A time missed while the machine was off or asleep runs once when it is next on, never once per missed time.
- Scout runs happen one at a time, whatever started them. A run that finds another in progress waits, and says so.
- Pausing a scout keeps its schedule; turning the schedule off keeps the scout.
- The database is the schedule. The alarms are rebuilt from it after every change and whenever the daemon starts; deleting a scout, its saved search, or all data removes its alarm.
- An alarm that could not be written, one the user switched off outside JobPilot, and a system with no scheduler JobPilot can use are each shown on the scout, which still runs by hand.
- What a scheduled run found, or that it could not run, is posted as a notification, as is any run that found a strong match (`docs/02_features/11_notifications.md`).

## Eligibility rules

What a Scout acts on, configured by the user:

- a minimum match score — on the Scout
- a ceiling on applications created per run — on the Scout
- companies never to apply to — on the saved search
- a salary floor — on the saved search
- locations or work arrangements to include or exclude — on the saved search

The last three are not duplicated onto the Scout.
`SearchCriteria` already holds them and `SearchCriteria.Matches` already applies them everywhere else in the product, so a copy would be a second place to change one rule with no answer for which wins (ADR-0018 §1, following ADR-0015 §4).

The checks run cheapest first, and the expensive one last: scoring calls the model once per job, so a job the saved search already excludes is never scored.
Measured on a real run over 50 stored jobs, one job matched and exactly one model call was made.

## Why nothing submits

An application is a representation by the candidate, about themselves, under
their name.

In Review mode a wrong answer is caught by the person looking at the filled
form, and the cost of a miss is an awkward moment before they send it.
Submitted unattended, the same wrong answer is irreversible, attributable to
a named real person, and discovered by them only after an employer has read
it.

The safeguards the old Autopilot spec relied on — a CAPTCHA stops
automation, an unknown required question stops automation, an ambiguous
field stops automation — are each a reason the agent should not submit.
Assembling them into a feature that submits when none of them fires assumes
the list is complete.
The defects found while building ADR-0013 argue against assuming that: a
newsletter box was read as an application form, an invisible reCAPTCHA was
read as a blocking challenge, and a label made of a UUID was offered to the
user as a question.
Each was found by looking, not by the checks that were already there.

## Acceptance Criteria

### AC-001
A Scout creates applications only for jobs that clear its eligibility rules.

### AC-002
A Scout never submits an application, and no configuration makes it do so.

### AC-003
A Scout run contacts no employer: no browser is opened and no application
form is reached.

### AC-004
A Scout run is recorded as an agent run, with what it considered, what it
rejected and why, and what it created.

### AC-005
A Scout honours its per-run ceiling.

### AC-006
A run that fails partway leaves the applications it already created intact
and records the failure.

Cancellation stops further work, including when it arrives during the final job's scoring.
Final status persistence has a separate five-second deadline so a cancelled request can still be recorded as failed.
Failures to persist the run, last-run time, or activity events are logged without discarding completed applications or the partial report.

### AC-007
A scheduled scout runs at its scheduled time without JobPilot having been
opened since the machine started, and is recorded as started by its
schedule.

### AC-008
A scheduled run happens only when the scout is enabled, still scheduled and
due. An alarm that fires early, twice, or for a changed or deleted scout
starts nothing.

### AC-009
A time missed while the machine was off or asleep runs once when it is next
on.

### AC-010
Two scout runs never overlap, so no job gets two applications from them.

### AC-011
Deleting a scout, its saved search, or all data removes its alarm from the
operating system. Saving a scout whose alarm cannot be written saves the
scout and says why it is not scheduled.

## Deferred

**Unattended preparation in the browser.**
Waking up to filled forms is the natural next step.
It needs the browser worker's tab contract resolved first: filled answers
persisted and the window re-opened on demand, rather than every run leaving
a window behind. It also needs an answer for boards that want a login, which
a Scout running unattended cannot provide.
