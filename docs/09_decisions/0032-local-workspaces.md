# ADR-0032: Isolating local profiles as workspaces

## Status

Accepted on 2026-10-08.

## Context

JobPilot was built around one candidate profile per installation.
Some records carry `candidate_id`, but settings, saved searches, Scouts, watched companies, credentials, notifications, jobs and browser state are installation-wide.
The runtime also obtains the first candidate row whenever it needs “the” profile.

Adding a profile identifier to selected screens would therefore create accidental data sharing.
Adding it to every table and query would turn the existing repositories into a tenant-enforcement layer whose one missed predicate could expose another profile's data.

The requested behaviour is several Gmail- or Slack-like profiles with different settings and job-search history, while preserving the product's local-first and no-account constraints.

## Decision

### 1. The persistence boundary is a workspace

Each local workspace owns one SQLite database and one browser profile directory.
The schema inside that database remains the schema for one candidate and their search.

The UI calls a workspace a profile.
Core and adapters use `Workspace` because `Candidate` is already the professional profile used for matching and form filling.

### 2. The installation keeps a small workspace catalog

The application data root holds an atomically replaced `workspaces.json` catalog.
It contains workspace identifiers, display names, creation/update times and the active workspace identifier.

The catalog contains no candidate facts or secrets.
Workspace identifiers are generated UUIDs and are validated before they participate in a filesystem path.

The daemon lock, discovery file and log remain installation-global because one daemon serves every workspace.
The desktop theme also remains installation-global.

### 3. Existing data is adopted in place

When no catalog exists, the root `jobpilot.db` and `browser-profile` become the first workspace.
They are not moved: a live SQLite database may also have WAL and shared-memory files, and moving that set is not one atomic operation.

New workspaces live under `workspaces/<id>/`.

### 4. One daemon dispatches to isolated runtimes

The daemon owns one runtime per registered workspace.
Each runtime has its own database connection, services, model settings, credentials namespace and browser session.

Existing feature handlers remain single-workspace handlers.
A top-level router handles workspace management and delegates other requests according to `X-JobPilot-Workspace`.
An omitted header uses the catalog's active workspace for backward compatibility.
An unknown identifier is rejected before a workspace path or service is reached.

### 5. Switching is explicit client state

The desktop records the active workspace through the daemon and sends its identifier on data requests.
It clears its query cache and remounts workspace-owned UI when switching.

The CLI accepts a workspace selector for one invocation.
Selecting one for a CLI command does not change the desktop's active workspace unless the user explicitly runs the activation command.

### 6. External state is namespaced

Credential values remain in the OS keychain and remain write-only through the API.
The internal keychain account includes the workspace identifier, so identical public `(source, label)` references do not collide.

Each workspace has its own Chrome profile and therefore its own job-board sign-ins.

Scout alarm identifiers and wake commands include the workspace identifier.
Reconciliation for one workspace must not remove another workspace's alarms.
A scheduled wake carries the workspace explicitly, so it never depends on whichever workspace the desktop currently shows.

### 7. A workspace is not an account or a security principal

There is no sign-in, password, remote identity or authorization boundary.
Anyone who can read the operating-system account's JobPilot data can read its workspaces.
People who need security separation must use separate operating-system accounts.

## Consequences

- Existing repository methods do not need a tenant predicate added to every query.
- Opening several databases costs more file handles and SQLite connections, but the number of personal workspaces is expected to be small.
- Shared job deduplication across workspaces is deliberately traded for isolation and simpler deletion.
- Device-global operations must not be stored in one workspace merely because that was the old location.
- “Delete all data” now means all data in the selected workspace; removing the workspace itself remains a separate future destructive operation.
- Deleting a workspace is a destructive operation over its database, browser state, credentials and alarms and must use the existing preview-and-confirm deletion pattern before it is exposed.

## Alternatives considered

**One shared SQLite database with `workspace_id` on owned rows.**
This shares the job catalog and keeps one connection pool, but requires changing nearly every table, unique constraint and query.
Its isolation depends on every future query remembering the workspace predicate.

**Treat each candidate row as a workspace.**
This leaves settings, searches, Scouts, credentials, notifications, job-interest state and browser sessions shared, which is not the requested behaviour.

**Run one daemon per workspace.**
This isolates process state but complicates discovery, upgrades, notifications and scheduled work, and spends resources on several idle processes.
