# ADR-0038: The model on the Exact apps

## Status

Accepted on 2026-11-16, by the user's instruction, with the spike below still to be run and its results recorded here.
Proposed on 2026-11-16.
It settles the question ADR-0037 §4 left open: which model scores a job on a phone, and where its key lives.

It builds on ADR-0030 (a cloud model) and ADR-0019 (storing a credential), and changes neither for the Go daemon.
It would replace the open question in ADR-0037 §4 for the mobile apps only.

## Context

The scoring pieces are ported to TypeScript in `exact-ui/core/`: the prompt, the strict reply parser, and the Ollama and OpenAI-compatible providers over an injected `fetch`.
No app code calls them yet, because the app has no model settings and no place for a key.
Until it has, a scout run records that jobs could not be scored and creates no applications, which is the Go rule: an unscored job is never eligible.

### The cloud question is already decided for the daemon

ADR-0030 accepted a hosted model the user names: a base URL, a model, and an API key kept in the keychain under the source `llm`.
It also decided that a key is sent only over HTTPS or to this machine, that errors are written from the status and never from the body, and that the user is told where their data goes.
None of that depends on the daemon.
What is undecided is how it carries over to a phone.

### Four things differ on a phone

**Loopback is the phone.**
`localhost` on a phone is the phone itself, so an Ollama on the user's computer is reached by a LAN address or not at all.
That address is one the user types, so the app's network grant cannot name it at build time.
Exact has two grants for it: `net.fetch https:` (any host, for a server the person names at run time) and `net.fetch local-network` with a purpose string.
Either widens what the app may reach, far beyond today's list of job-board origins.

**A secret's name is fixed at build time.**
Exact keeps a secret with `secret.keep <name>`, one grant line per name, and reads it with `store.get(name)`.
ADR-0030's labels are the user's (`openrouter`, `work`), so a name per label cannot be granted in advance.

**The secret store differs by host.**
It is the Keychain on Apple.
On the web it is `localStorage`, readable by any script on the origin, and on Linux unencrypted files in the app's data directory.
Exact's documentation does not say what Android uses.
The web host is a development target for this app, not a place to keep a real key.

**A phone cannot run a scout in the background.**
A wake lasts at most 30 seconds, no sooner than every 15 minutes (ADR-0037 §3).
A scout that scores jobs with a hosted model therefore runs only while the app is open.

### Where the data goes

Scoring sends the profile and the job posting to the model.
With a hosted model, all of it goes to that provider, every time a run scores a job.
`AGENTS.md` says user data stays local by default, so this must be the user's choice, shown to them when they make it, as ADR-0030 §6 already requires.

## Decision

### 1. The model is the one the user names

A phone's model is a provider the user configures: an OpenAI-compatible endpoint, or an Ollama server.
ADR-0030 applies unchanged to the app: the base URL as the provider publishes it, no key over plain `http` unless the host is this device, errors from the status only, and the host shown beside the field with what is sent there.
These are deterministic rules in core, in `core/llm-settings.ts`, with the Go test's cases.

No provider or model is a default.
Until the user configures one, nothing is scored, and the Scouts screen says so.

### 2. Settings in SQLite, the key in one Keychain secret

The settings (provider, base URL, model, the key's label) are rows in the app's SQLite, as `core/llm-settings-store.ts` stores them.
A key is never in SQLite.

Because a secret's name is fixed at build time, the app grants one name, `llm-keys`, and keeps a JSON object in it: `{ "<label>": "<key>" }`.
Reading a key is `store.get('llm-keys')`, then the label's entry, for the length of one request.
The settings hold the label only, as ADR-0030 §1 requires, and the screen never shows a key again after it is stored.

The key is read only through the one module that builds a request, so no screen handles it.

### 3. The network grant is the user's to widen

The app ships with the origins of its job sources and the two common hosted model hosts as named grants only if the user picks one; it does not ship `net.fetch https:` by default.
Choosing a custom endpoint is the point at which the app needs a broader grant.
Whether Exact lets an app ask for one at run time, or whether the build must carry it, is not verified; it is the first thing the spike checks.
If it must be built in, the choice is between shipping `net.fetch https:` for everyone and shipping a build without custom endpoints.
This ADR prefers the second until the first is justified, because it keeps the app's reach as small as the rest of ADR-0037's work.

### 4. A run scores while the app is open

A scout run that scores with a hosted model runs in the foreground and can be cancelled, through the `AbortSignal` the providers already take.
A periodic wake may fetch and notify, never score (ADR-0037 §3).

### 5. The on-device model stays a spike

An on-device model (ADR-0037 names NobodyWho, EUPL-1.2) is not decided here.
It needs three answers: whether an Exact data crate can link an inference engine on iOS and Android, how large a model the user must download and how slow it runs, and whether its licence can ship with JobPilot.
The provider port in core is the seam: an on-device model is another provider behind it.
Until the spike answers, a phone has no model of its own.

## Consequences

- The scout run on a phone can score, once settings and a key exist, without any server of JobPilot's.
- The app's network reach grows only when the user names a custom endpoint, and the cost of that is stated in §3.
- One Keychain secret holds every key, so removing one label rewrites the object, and the web host shows the keys to any script on its origin.
  The settings screen says the web host is for development.
- A user without a model sees a Scouts screen that finds and filters jobs but scores none.
- ADR-0037 §4's open question is replaced for the apps: cloud by the user's choice now, on-device by a spike.

## Open questions

- **Android's secret store.** Exact's documentation names the Keychain, the web and Linux.
  What `store.set` uses on Android is unverified, and a key must not be stored on a host that keeps it in the clear.
- **The run-time grant.** Whether an app can be granted `net.fetch https:` or `local-network` after install, or only at build.
- **Cost and quota.** A run scoring many jobs can spend a user's quota quickly.
  The Go scout has a per-run ceiling; whether the app also needs a per-day spend guard is undecided.
- **One key at a time.** `llm-keys` allows many labels, but the settings name one.
  Whether the app needs more than one is open.

## Spike

The user accepted this ADR before the spike ran, so the spike now confirms it and its results are recorded here.
Answered from Exact's reference (2026-11-16): a network grant is a line in the app's `grants`, fixed at build time.
`net.fetch https:` is the only grant that reaches a host the person types, and nothing asks for it after install.
So §3's first branch is closed: a custom endpoint needs `net.fetch https:` in the build, and the app ships without it until a decision justifies it.
Open still: Android's secret store, and a real key saved and read on a device.

On iOS and Android, store a key in the `llm-keys` secret and read it back, then send one scoring request to an OpenAI-compatible host the user names, through `core/match.ts`, and show the score.
Record what the Android secret store is, and whether the custom-endpoint grant can be asked for at run time.

## Alternatives considered

- **An on-device model only.** Nothing leaves the phone, but the size, speed and linking questions are unanswered, and a phone with no model could score nothing.
- **A JobPilot-hosted proxy for the model.** Rejected: JobPilot has no mandatory cloud backend (AGENTS.md).
- **The desktop daemon's model, over the network.** Keeps one model setting, but needs the computer on and reachable, which ADR-0037 rejects for the whole core.
- **A secret per label.** Matches ADR-0030's labels, but the grant list is fixed at build time, so a new label would need a new build.
- **The key in SQLite beside the settings.** Rejected by ADR-0019 option B.
