# Feature: Agent Activity

## Purpose

Show transparent, structured activity without exposing hidden chain-of-thought.

## Show

- run ID
- timestamp
- action
- tool
- status
- result summary
- policy decision
- errors

## Do not show

- hidden chain-of-thought
- secrets
- unnecessary sensitive candidate data

## What is recorded

Two kinds of work leave a trail today: preparing an application in the browser (`prepare_application`) and a scout run (`scout_run`).
Matching and cover letters do not create runs.

A run names what it served.
A browser run belongs to one application; a scout run belongs to none, and records its scout instead (migration `00012_agent_activity.sql`).
Deleting the scout or the application keeps the run and clears the link, so the record of what happened outlives the thing it happened to.

The policy result of a step is the verdict of a deterministic rule, `allowed` or `denied`, with the reason in the step's output:

| step | rule |
| --- | --- |
| `job_rejected`, `application_created`, `ceiling_reached` | a scout's eligibility rules (ADR-0018) |
| `extract_form` | the application-form gate (ADR-0013) |
| `detect_blocker` | JobPilot never gets past an anti-bot measure (ADR-0014) |
| `skip_field` | an answer is not one of the choices a field offers |

A step no rule decided has no policy result.

A filled field's step records the field's label and nothing else.
The answer that was typed is stored on the application's question, which is where the user reviews it; a copy in the trail would put an email address, a phone number or a work authorization on a screen that has no reason to show them.
Trails written before this rule had their answers removed by the same migration.

## Where it is shown

- The desktop's Activity screen lists every run, newest first, filterable by kind and status, and opens each one to show its steps.
- The Application detail screen lists that application's runs, and the Scouts screen lists scout runs; both link to the run in Activity.
- `jobpilot-cli activity list` and `jobpilot-cli activity show <run-id>` read the same trail.

## Acceptance Criteria

### AC-001
Every agent run, of every kind, is listed in one place, newest first, with its kind, what it served, status, start time, duration and step count.

`GET /v1/agent-runs`, paged by `limit` and `offset`, filtered by `task_type` and `status`.

### AC-002
A run can be opened to show every step it recorded, in order, with action, tool, status, input and output summaries, policy result and duration, and the run's error if it stopped.

`GET /v1/agent-runs/{id}`; an unknown run is `404`.

### AC-003
A filter that names an unknown kind or status is refused, not answered with an empty list.

### AC-004
No step records an answer typed into a form.

### AC-005
A step decided by a deterministic rule records that rule's verdict as its policy result.

### AC-006
The CLI can list runs and show one, as the desktop can.
