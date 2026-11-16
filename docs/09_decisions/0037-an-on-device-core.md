# ADR-0037: An on-device core for the Exact apps

## Status

Proposed on 2026-11-16.
The user asked on 2026-11-16 whether the Go daemon's behavior could be rewritten, not linked, inside Exact, so that the iOS and Android apps need no daemon process on a computer and no server.
This ADR records what is known, what is not, and the spike that would settle it.

It would supersede ADR-0012 (the loopback HTTP API) for the mobile apps only, and partly change the "Prefer Go for core" constraint in AGENTS.md.
Nothing is superseded until this is accepted.
The Go daemon remains the core of the desktop product.

## Context

The daemon is already local: it listens on loopback and keeps SQLite on the user's machine (ADR-0012).
No hosted server exists today.
A phone cannot run it, because iOS and Android do not let an app start or keep a separate daemon process.
So a phone app needs its core inside the app.

Exact apps can hold that core.
Their data layer is TypeScript (`app.ts`) or a Rust crate, with grants such as `sqlite.open`, `fs.read` and `net.fetch`.
Both run on iOS and Android without a server.

## Decision

### 1. The mobile apps carry their own core

Discovery, matching rules, the application state machine, and storage are reimplemented over on-device SQLite.
The first slice is TypeScript in Exact's data module, not Rust: it already reaches `fetch` and SQLite on every host, and its rules run under `bun test` with no Exact at all.
Rust (a data crate) stays open for what TypeScript cannot do, such as linking an inference engine.
It is a port of behavior, not a link to the Go code.
The Go daemon and its tests stay the reference for what each rule does, and the port is checked against the same fixtures.

### 2. Deterministic rules are ported once, and tested against the Go cases

Eligibility, the state machine, salary and location filtering, and the scout ceiling are deterministic and live in core (AGENTS.md).
Each ported rule is covered by the Go test's cases, so the two cores cannot drift silently.

### 3. Scouts do not run on a schedule on a phone

Exact's background work gives a wake of at most 30 seconds, no sooner than every 15 minutes, at the platform's choice, which on iOS can be hours.
A scout run (fetch, match, create applications) does not fit that reliably.
On a phone, a scout runs when the user opens it or asks it to, and may use a periodic wake only to fetch and notify.
The scheduled scout stays a desktop feature.

### 4. The model is local or a provider the user names

The provider abstraction (ADR for LLM access) stays.
Two providers apply on a phone: a cloud provider the user configures, and an on-device model.
NobodyWho (https://github.com/nobodywho-ooo/nobodywho) is a candidate for the second.
It runs GGUF models on iOS and Android, with tool calling, and has Swift, Kotlin, React Native and Flutter bindings.
Its core is Rust, but it documents no Rust crate and no Exact integration, so whether an Exact data crate can depend on it is unverified.
Its licence is EUPL-1.2, which needs a review before JobPilot ships it.

## Spike findings

The slice is in `exact-ui/core/`, with `bun test` coverage, and a Discover tab in the app.
It ports the search rule (`SearchCriteria.MatchesFor`, minus work-from and fit-hours) and the Remotive source, keeps the listing in SQLite for four hours, and shows the matches, with no daemon.

- The application state machine and the apply-link check are ported too (`core/application.ts`), with the Go test's transition cases.
- It passes on the web host, including SQLite and the offline-after-fetch path.
- It has not been built for iOS or Android: this machine has Xcode 27.0 where Exact needs 27.1, and lacks Android SDK platform 37.2 and a JDK without JDK-8335662.
- A data source cannot read the clock, so the view passes the current hour in.
  Staleness is decided at hour resolution, and the Go core's `time.Now()` rules (the four-hour cache, a scout's next run) need the same treatment.
- Remotive's HTML description is stripped to text, not converted to Markdown as in Go.

## Open questions

- **Linking an inference engine.** Can an Exact Rust data crate depend on NobodyWho or llama.cpp on iOS and Android?
- **Model size and the 30 second wake.** A local model downloads gigabytes and is slow on a phone, so matching may be foreground-only.
- **Job sources.** The sources are HTTP APIs, so they port directly.
  Reading a page that has no API needs a browser, and Playwright has no phone equivalent.
- **Two cores.** The Go and Rust cores must be kept in agreement for as long as both ship.
- **Sync between devices.** None exists; each install has its own data (ADR-0001, local-first).

## Spike before acceptance

Build one vertical slice in Rust inside `exact-ui`: fetch jobs from one source API, store them in on-device SQLite, apply the eligibility rules, and show them, on iOS and Android, with no daemon.
Then call one cloud LLM through the provider abstraction for a match.
The result decides whether the port is worth the cost.

## Consequences

- The mobile apps become independent of any computer.
- JobPilot carries two implementations of the core.
- The scheduled scout is not on mobile.

## Alternatives considered

- **Link the Go code with gomobile.** Not verified to work with Exact's hosts, and the user declined linking in favor of a rewrite.
- **A hosted server.** Rejected: JobPilot is local-first with no mandatory cloud backend.
- **A phone that talks to the desktop daemon.** Keeps one core, but needs the computer to be on and reachable.
