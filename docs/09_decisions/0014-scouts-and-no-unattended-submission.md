# ADR-0014: Scouts replace Autopilot mode, and nothing submits

## Status

Accepted.
Supersedes ADR-0007 ("Three Application Modes"), which stands as the record of what was decided before and why.

## Context

ADR-0007 accepted three application modes — Assist, Review and Autopilot — on the rationale of "progressive automation while preserving human control over consequential actions".

Autopilot as written fuses two separate ideas:

1. **Unattended work.** The agent goes and does something useful without the user driving each step.
2. **Automatic submission.** The agent sends an application to an employer with nobody watching.

They have been treated as one feature because they arrived in one sentence.
They have different value and completely different risk, and separating them is most of this decision.

There is also a structural problem.
Autopilot is modelled as `domain.ApplicationMode` — a field on a single application.
But "watch my saved searches and bring me things worth applying to" is not a property of an application; it is a standing instruction that *produces* applications.
Nothing about it fits on the record it creates, which is why it reads as out of place on the create-application flow.
It was never a mode.
It was a pipeline in the only box that existed when ADR-0007 was written.

Nothing of it is built.
It is one enum value (`domain.ApplicationModeAutopilot`), a CLI flag string, a TypeScript union member and a label in the desktop, plus 21 mentions across 12 documents.
Roadmap Phase 8 — policy engine, policy evaluation, eligibility rules, submission safeguards, notifications — is unstarted, and exists almost entirely to make automatic submission safe.

Meanwhile the architecture has been growing away from submission, deliberately and on the record.
`docs/01_architecture/03_browser_worker.md` has a "Submission boundary" section saying the worker never clicks the target site's own submit control, because leaving the filled-in window open for the human *is* what satisfies Review Mode's explicit-approval requirement.
That is a structural property rather than a procedural one: there is no code path that submits, so there is no code path that can submit wrongly.

What *does* exist to build unattended work on is most of the pipeline.
Saved searches carry criteria, match analysis runs against a candidate and a job, application creation is a service call, and `AgentRun`/`AgentEvent` already record what an agent did and why.
ADR-0009 anticipated this: the daemon "may start for scheduled work".
No scheduler exists yet.

## Decision drivers

- An application is a representation by the candidate, about themselves, under their name (core principle 6: never invent candidate facts).
- Unattended work is valuable in direct proportion to how little it can do that cannot be undone.
- Deterministic business rules must not be delegated to an LLM (core principle 7).
- The user explicitly approves consequential external actions (core principle 5).
- An unattended run is exactly the case where "what did it do while I was asleep" has to be answerable.

## Decision

Split the two ideas. Keep one, remove the other, and move the one we keep to where it belongs.

### 1. JobPilot does not submit applications to external systems

There is no mode, setting, policy or configuration that causes it to.
The submission boundary is promoted from a property of the browser worker to a product invariant:

> No JobPilot code path clicks an external application's submit control.
> The user submits, in a browser window JobPilot has filled in and left open for them.

This belongs with the core principles in `AGENTS.md`, because it is now the kind of constraint the others are — something an implementation may not quietly change.

The existing rule "never submit applications without the user's configured authorization" becomes "never submit applications".
A rule that cannot be satisfied by configuration is a rule that is easier to hold.

Every safeguard ADR-0007 relied on — CAPTCHA stops automation, unknown required questions stop automation, ambiguous fields stop automation — is a reason the agent should not submit.
Assembling them into a mode that submits when none of them fires assumes the list is complete.
The defects found while building ADR-0013 argue against assuming that: a newsletter box was read as an application form, an invisible reCAPTCHA was read as a blocking challenge, and a label made of a UUID was offered to the user as a question.
Each was found by looking, not by the checks that were already there.

### 2. Autopilot stops being an application mode

Application modes become **Assist** and **Review**, with Review the default.
`ApplicationModeAutopilot` is removed from the enum, the CLI flag, the desktop's type union and its label.

This is not a reduction in capability, because the capability moves.

### 3. Unattended work becomes a Scout

A **Scout** is a standing instruction the user configures once, separate from any application.

One scout run:

1. Resolves its saved search and its eligibility rules.
2. Obtains jobs to consider — the jobs already stored locally, or more fetched from the configured sources, as the scout is set up.
3. Analyzes each against the candidate, through the existing match service.
4. Creates an application, in the ordinary initial state, for each job that clears the rules.
5. Records the whole run, and every decision in it, in the agent trail.

The output is a shortlist of applications waiting for the user.
The user opens one, reviews the match, and prepares it in the browser themselves — the flow that exists today, entered from a list someone else assembled.

**A scout run stops before the browser.**
It contacts no employer.
The only external traffic is the job-source APIs the application already calls.

That boundary is chosen rather than incidental:

- The browser worker leaves a tab open per prepared application on purpose, because a human is waiting on it. Unattended, twenty jobs is twenty Chrome tabs. Changing that means persisting filled answers and re-opening the window on demand, which is a change to the worker's contract, not a setting.
- Boards behind a login need a person. The user logs into each board once inside JobPilot's own Chrome profile, and a scout running at 3am cannot do that.
- Reaching a form is not always without effect on the employer's side. Some applicant tracking systems create an account or save a draft record. Unattended and at volume, that is traffic and state nobody decided to create.

Unattended preparation in the browser is deferred, not refused. See "Deferred" below.

### 4. Phase 8 becomes eligibility rules, not submission safeguards

The policy engine is not deleted, it is re-aimed at deciding what a scout acts on:

- a minimum match score
- companies never to apply to
- a salary floor
- locations or work arrangements to include or exclude
- a ceiling on applications created per run

These stay deterministic and stay in core, exactly as `docs/02_features/10_autopilot.md` already required of policy rules, minus the part where their output authorizes an irreversible external action.

This makes the phase both smaller and more testable.
A rule that filters a list can be checked against a list.
A rule that authorizes a submission can only be checked against a submission.

### 5. A scout run is an agent run

Scouts reuse `AgentRun`/`AgentEvent` rather than inventing a parallel history.
Unattended work is precisely the case those records were built for, so this is the first use that actually needs them: the user must be able to ask what ran, what it considered, what it rejected and why, and what it created.

## Consequences

- `domain.ApplicationMode` loses `autopilot`, and with it the CLI flag value, the TypeScript union member and the desktop label.
- Existing rows can hold it, which was not obvious.
  `applications.mode` is a free `TEXT` column with no constraint, and `ApplicationService.CreateApplication` stores any non-empty string it is given without checking it against the constants, so `application create --mode autopilot` persists that value today.
  It has never meant anything — no code reads the mode to decide behaviour — but the rows can exist, so a migration rewrites any `autopilot` to `review`.
  Review is the correct destination rather than a lossy one: it is what those applications have actually been getting.
- A related gap needs closing in the same change: an unrecognised mode should be rejected at the boundary rather than stored, so a typo cannot silently create a fourth mode — and so the removed one cannot come back by accident.
- `docs/02_features/10_autopilot.md` is replaced by a Scout feature specification rather than deleted, carrying the declined-submission reasoning with it. A reader who finds the mode gone deserves to find out why where they would look first.
- Twelve documents lose or rewrite their Autopilot sections, including `docs/00_product/04_application_modes.md`'s invariants, `docs/00_product/03_user_flows.md`'s Autopilot flow, and acceptance criterion `AC-G-019` ("Autopilot cannot bypass policy restrictions"), which becomes unconditional.
- Roadmap Phase 8 is re-titled and re-scoped from Autopilot to Scouts and eligibility rules.
- ADR-0007's rationale, "progressive automation while preserving human control over consequential actions", survives intact. The progression now ends one step earlier, at a filled form the human sends.
- The rationale for normalizing candidate entities changes and does not disappear. It was partly "a policy engine should not reason over JSON blobs"; it becomes answer resolution — deterministically answering the work-authorization, notice-period and years-of-experience questions real forms ask, from structured data rather than by pattern-matching a text blob. The ADR for it should say so rather than inheriting the old justification.
- JobPilot gives up the "apply to hundreds of jobs unattended" market outright. That is a real narrowing of ambition, chosen rather than stumbled into.

## Deferred

**How a scout is triggered.**
Manual "run now" is enough to build the thing and prove it.
Scheduling is a separate question, and ADR-0009 constrains it: there is no 24/7 daemon, so a schedule is either the desktop application starting work when it is open, or an OS-level scheduler starting the daemon.
That deserves its own decision rather than being settled by whichever is easier to write.

**Unattended preparation in the browser.**
Waking up to filled forms is the natural next step and the reason to keep the door open.
It needs the browser worker's tab contract resolved first — filled answers persisted and the window re-opened on demand, rather than every run leaving a window behind — plus an answer for boards that want a login.
Neither is hard; both are decisions, and neither is required to make scouts useful.

## Alternatives considered

**Keep Autopilot including submission, gated by the policy engine.**
This is ADR-0007 as accepted.
It requires building the one capability the design deliberately lacks, then building Phase 8 to contain the risk that introduces, and it puts the cost of a mistake on the user's name rather than on the software's reputation.

**Remove Autopilot entirely, unattended work included.**
The first draft of this ADR.
It threw away the valuable half to be rid of the dangerous half.
Unattended discovery, matching and shortlisting carries no irreversible action at all, and removing it would have been a real loss justified by nothing.

**Keep Autopilot as an application mode, minus submission.**
Leaves the pipeline modelled as a field on the records it creates, so the setting has no coherent home and no way to express a schedule, a source scope or a per-run ceiling.
The mismatch is the original mistake, not a cosmetic one.

**Submit unattended, but only after a review timeout.**
Unattended submission with a delay.
It fails in exactly the case that matters: the user who is asleep, away, or trusting it.
