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

One kind of work leaves a trail: a scout run (`scout_run`).
Matching does not create runs.
Preparing an application in the browser (`prepare_application`) left a trail until ADR-0034 removed it; migration 00026 deleted those runs with their steps.

A run names the scout that ran (migration `00012_agent_activity.sql`).
Deleting a scout keeps its runs and clears the link, so the record of what it did outlives it.

The policy result of a step is the verdict of a deterministic rule, `allowed` or `denied`, with the reason in the step's output:

| step | rule |
| --- | --- |
| `job_rejected`, `application_created`, `ceiling_reached` | a scout's eligibility rules (ADR-0018) |

A step no rule decided has no policy result.

A run is `running`, or finished as `completed` or `failed`.
A run still `running` when its workspace opens was cut off by a daemon that stopped without closing it, and is closed as `failed`.

## Where it is shown

- The desktop's Activity screen lists every run, newest first, filterable by status, and opens each one to show its steps.
- The Scouts screen lists scout runs, each linking to the run in Activity.
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
No step records the candidate's answers or profile details.

### AC-005
A step decided by a deterministic rule records that rule's verdict as its policy result.

### AC-006
The CLI can list runs and show one, as the desktop can.
