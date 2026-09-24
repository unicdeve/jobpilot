# ADR-0018: What a Scout is made of

## Status

Accepted, and implemented in full.

This is the data model for the Scout that ADR-0014 decided on and `docs/02_features/10_scouts.md` specified.
It changes nothing about what a Scout does or refuses to do.

## Context

ADR-0014 §4 re-aimed Phase 8 at eligibility rules and listed five:

- a minimum match score
- companies never to apply to
- a salary floor
- locations or work arrangements to include or exclude
- a ceiling on applications created per run

Building them turned up three things worth writing down before any code exists.

### Three of the five are already built

`SearchCriteria` already carries `SalaryMin`, `ExcludedCompanies`, `Location`, `RemotePreference` and `EmploymentType`, and `SearchCriteria.Matches` already applies every one of them.
It is the single definition of what a search means, used by `JobDiscoveryService` for every source that cannot filter server-side.

ADR-0015 §4 put the salary floor there deliberately, on the grounds that a floor is a filter and a filter belongs to the search.
The same argument covers the other four.

So a Scout that carried its own salary floor and its own company blocklist would be a second place to say a thing already said, with no answer to which one wins.

### The `policies` table is unused Phase 0 schema

Migration `00001` created it:

```sql
CREATE TABLE IF NOT EXISTS policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    rule TEXT NOT NULL,
    ...
);
```

Nothing has ever read or written it.
`rule TEXT` is an untyped blob standing in for a decision nobody had made yet.

This is the same shape as the columns ADR-0015's closing change removed — `work_preferences`, `education`, `certifications` — schema the implementation never grew into, which reads as though something uses it.

### A scout run cannot be an agent run yet

`agent_runs.application_id` was added by migration `00003` as `TEXT REFERENCES applications(id)`, and foreign keys are on (`_foreign_keys=on` in the DSN).
`AgentRunRepository.Save` writes `run.ApplicationID` directly rather than through `nullableString`.

A scout run belongs to no single application — creating several is the point — so it would write the empty string, and the foreign key rejects it.
Verified: inserting `''` fails with `FOREIGN KEY constraint failed`, and `NULL` succeeds.

AC-004 requires a scout run to be recorded as an agent run, so this is a blocker rather than a detail.

## Decision

### 1. A Scout is a saved search plus what a search cannot express

```
Scout
  name
  saved_search_id     -> the criteria, applied by SearchCriteria.Matches
  min_match_score     new: a search cannot judge fit
  max_per_run         new: a search has no notion of a run
  source              new: local jobs only, or fetch more first
  is_enabled
```

Salary floor, excluded companies, location and work arrangement come from the saved search.
There is one place to change a rule, and it is the place that already applies it everywhere else.

The cost is stated rather than hidden: a user cannot yet say "show me jobs from €60k but only shortlist above €80k".
Nothing in the product asks for that today, and splitting a threshold in two is easy to add later and hard to remove once someone depends on it.

### 2. `policies` is dropped, and a typed `scouts` table replaces it

A Scout's rules are four typed columns, not a serialized rule language.
Four columns can be read in a `SELECT`, constrained by the schema and checked by the compiler; a `rule TEXT` blob can be none of those.

ADR-0014 said the policy engine is "not deleted, it is re-aimed".
That is what this is: the idea survives, the empty table does not.

### 3. Eligibility is one function in core, and it explains itself

A single deterministic function decides whether a job earns an application, and returns *why* rather than a boolean.

The reason is not decoration.
AC-004 requires a run to record what it rejected and why, and a rejection reason that is derived separately from the decision is a second implementation that can disagree with the first.

Checks run in this order, cheapest and most certain first:

1. the job matches the saved search
2. an application does not already exist for it
3. the match score clears the floor
4. the run's ceiling has not been reached

An LLM decides none of this (AGENTS.md principle 7).
The match *score* comes from the model, as it already does; whether a score clears a floor is arithmetic.

### 4. A scout run is an AgentRun with no application

`AgentRunRepository.Save` writes `application_id` as NULL when it is empty, which is what the column already allows and what every scout run needs.

`AgentRunRepository` gains a way to list runs that belong to no application, so the desktop can show scout history.

### 5. Scheduling stays deferred

`10_scouts.md` defers it and ADR-0009 constrains it.
A Scout runs when the user asks it to.

## Consequences

- Phase 8 is smaller than ADR-0014 implied, because three of its five rules were already built and the fourth and fifth are two integers.
- A fix to `AgentRunRepository.Save` that no current caller needs: every existing run has an application. It is required by the first caller that does not.
- One migration drops a table. It has never held a row, so nothing is lost, but it is still a table drop and is called out here rather than buried.
- A Scout depends on a saved search, so deleting a search must decide what happens to Scouts using it. The foreign key cascades: a Scout without its search has no criteria, and a Scout that silently matched everything instead would be the worst possible failure.

## Alternatives considered

**Duplicate the rules onto the Scout.** Considered and rejected above: two homes for one rule, and no answer for which wins. It buys a split threshold nobody has asked for.

**Keep `policies` and store the rules as JSON in `rule`.** This is what the table was shaped for. It trades every guarantee the schema and the compiler provide for a flexibility that a four-field record does not need, and ADR-0015 spent a whole change undoing exactly this pattern elsewhere in the product.

**Give scout runs their own table.** Rejected by ADR-0014 §5: unattended work is the case `AgentRun` was built for, and a parallel history would have to be merged back together for the user to read.
