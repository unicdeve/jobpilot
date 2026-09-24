# ADR-0019: Storing a credential

## Status

Accepted. Not yet implemented.

Both open questions were settled by the user when the proposal was read: the API is write-only, and a credential is identified by source **and** label.
The second changed the design rather than just confirming it — see §1 and §6.

This implements the "Secret storage" line of `docs/01_architecture/08_security.md`, which has been unimplemented since it was written because nothing needed it.

It does not change ADR-0012's transport, but it does narrow what that transport is allowed to return.

## Context

JobPilot stores no credential today, anywhere, of any kind.

That is not an oversight, and it is worth being precise about why, because two of the three places one would expect a secret have already been solved without storing one.

### Job board logins are solved by a browser profile

The browser worker runs Chrome against a persistent JobPilot-owned profile, never the user's daily driver.
The user signs into a board once, by hand, in a window they can see, and Chrome keeps the session.
JobPilot never sees a password and never needs one.

This is the better answer, not a workaround: a stored password would let an unattended run authenticate as the candidate, and ADR-0014 is built on the principle that nothing acts as them unattended.

### The cloud LLM credential belongs to Ollama

The four configured providers are Ollama, llama.cpp, LM Studio, and any OpenAI-compatible endpoint, all reached over a local base URL, none authenticated.
`domain.LLMSettings` has three fields — provider, base URL, model — and no key.

The user's own configured model is `gemma4:31b-cloud`, an Ollama cloud model.
The credential for it is held by Ollama, which they authenticated once with `ollama signin`; JobPilot talks to `localhost:11434` and never learns it.

Again the pattern is delegation: the tool that needs the secret holds the secret.

### What is left is API keys for job sources

This is the real need, and it is narrower than "secrets" in general.

Measured, with no credentials supplied:

| API | response |
| --- | --- |
| Adzuna | 400 |
| Jooble | 403 |
| USAJOBS | 403 |
| The Muse | 403 |

Google's Programmable Search JSON API is the one that prompted this — see `docs/02_features/04_job_discovery.md` for why scraping Google is not available — but it is one of a class.
Every source above is an established job API that a JobPilot user could reasonably want and cannot have, because there is nowhere to put the key.

So the question this ADR answers is not "how do we store secrets".
It is "how does a job-source connector get an API key", asked in a way that does not have to be redesigned when a second kind of credential appears.

### The problem that is not storage

ADR-0012 binds the daemon to `127.0.0.1` with **no authentication**, and explicitly deferred a per-instance token "until a concrete requirement exists":

> Loopback binding alone is the security boundary for MVP: only local processes can reach the port.

Every endpoint is therefore reachable by any process running as the user.

This matters more than the choice of keystore.
Putting a key in the OS keychain and then serving it from `GET /v1/settings/credentials` would take a secret that was protected by the keychain and make it readable over unauthenticated HTTP by any process on the machine.
That is worse than a file with `0600` on it, and it would be done in the name of security.

A credential that can be read back out is the concrete requirement ADR-0012 was waiting for.
This ADR avoids creating it rather than answering it.

## Decision drivers

- `docs/01_architecture/08_security.md`: "Use OS-native secure credential storage for API keys and other credentials."
- `AGENTS.md`: never store raw passwords in the application database; never log secrets.
- `AGENTS.md`: "Avoid infrastructure that is not justified by a documented requirement." One key, today.
- The CLI and the daemon must both work, including where no desktop session exists.
- The product is cross-platform, and the build is currently CGO-free on every target.

## Options

### A. Do nothing yet

Wait for a second credential before building anything.

Honest, and the status quo has served well. But it leaves a measured, wanted capability — a whole class of job APIs — blocked on infrastructure that will be needed eventually, and the shape of it is already clear.

### B. A column in SQLite

Rejected outright. `AGENTS.md` forbids it and `08_security.md` mandates otherwise.
Worth naming only because it is the path of least resistance and someone will propose it.

### C. An encrypted file in the data directory

Needs a key to decrypt the key. Either that key is derived from a passphrase the user types every start — which a local-first, no-account product has deliberately avoided — or it sits next to the file and protects nothing.

### D. OS-native keychain, write-only through the API

The keychain holds the value. The API accepts a credential and reports whether one is configured, and has no route that returns it. Connectors read it in-process, in the daemon.

## Decision

### 1. A credential is identified by a source and a label

```go
// A CredentialRef names one stored secret. Source is owned by the code that
// needs it; Label is the user's, so they can hold more than one.
type CredentialRef struct {
    Source string // "google-search", "adzuna"
    Label  string // "default", "work", "eu"
}
```

Both parts match `^[a-z0-9]+(-[a-z0-9]+)*$`: lower case, digits and single interior hyphens.
No separator, no whitespace, no empty part, no case to fold.

That restriction is what makes the pair composable without ambiguity.
`Source` and `Label` are joined with `/` to address the keychain item, and if either could itself contain `/`, then `adzuna` + `work/eu` and `adzuna/work` + `eu` would be the same item — one user's credential silently overwriting another's.
Validation is in the domain type, not at the HTTP edge, so no caller can construct a ref that does not round-trip.

A user who names no label gets `default`.

### 2. The store is a port

```go
type CredentialStore interface {
    Set(ctx context.Context, ref domain.CredentialRef, secret string) error
    Get(ctx context.Context, ref domain.CredentialRef) (string, error)
    Delete(ctx context.Context, ref domain.CredentialRef) error
}
```

Core depends on the interface, so a connector that needs a key asks for one and is testable with an in-memory fake, exactly as `ports.DocumentConverter` made `ResumeService` testable without a PDF engine (ADR-0016).

### 3. The adapter is `github.com/zalando/go-keyring`

Measured on macOS 26:

| | |
| --- | --- |
| set / get / delete | works, ~40 ms per call |
| a missing key | `ErrNotFound`, distinguishable from a failure |
| largest value stored | 3009 bytes |
| secret passed via | the child process's **stdin**, not `argv` |
| cross-compiles `CGO_ENABLED=0` | linux/amd64, windows/amd64, darwin/arm64 |

The stdin detail is the one that decides it.
The library drives `/usr/bin/security -i` and writes the command to its standard input, so the secret never appears in `ps` output.
An implementation that passed `-w <secret>` on the command line would leak it to every process on the machine for the life of the call, which is the sort of defect that is invisible in review and obvious in a measurement.

3009 bytes is far more than any API key needs — an Anthropic key is about 108 bytes, an 800-byte JWT stored fine — but it is a real ceiling, so `Set` must surface the library's error rather than assume success.

Three transitive dependencies, no cgo, and the CGO-free build is preserved on all three targets.
`99designs/keyring` was considered and is the wrong size: it brings backends for Vault, `pass` and encrypted files that this product has no requirement for.

### 4. A credential goes in and never comes out

The API gets:

- `PUT /v1/credentials/{source}/{label}` — store or replace
- `DELETE /v1/credentials/{source}/{label}` — remove
- `GET /v1/credentials` — the refs that are configured, and **no values**

There is deliberately no endpoint that returns a secret.
Nothing in the desktop application or the CLI needs to read one: the connector that uses a key runs inside the daemon, where it reads the store directly.

This keeps ADR-0012's "loopback is the boundary" honest.
A local process can still overwrite or delete a key whose ref it can guess, which is a nuisance, not a disclosure; reading one back would be a disclosure.

The desktop shows a credential as configured and never shows it again.
The UI must say so at the point of entry — "you will not be able to view this again" — because a field that silently turns into dots reads as a product that has lost the value, and the user's next move is to paste it again.

If some future feature genuinely needs to display a stored secret, that feature needs an authentication decision first, and it should reopen ADR-0012 rather than quietly adding a `GET`.

### 5. An environment variable overrides, and is never written

`JOBPILOT_CREDENTIAL_<SOURCE>__<LABEL>` is read first, and the keychain second.
Each part is upper-cased with hyphens turned into underscores, so `adzuna` + `work-eu` is `JOBPILOT_CREDENTIAL_ADZUNA__WORK_EU`.

The separator is two underscores because a single one is already what a hyphen becomes.
`^[a-z0-9]+(-[a-z0-9]+)*$` from §1 permits no consecutive hyphens, so no part can produce `__` on its own and the split is unambiguous.

This is how the product stays usable where there is no keychain: headless Linux without a running Secret Service, a container, CI. Those are real, and the alternative — inventing a file-based fallback store — is option C wearing a different hat.

An override is read-only. `Set` always writes to the keychain and never to the environment, so there is one place a credential is ever stored and no question about which copy won.

### 6. A table records which refs exist, and holds no secret

`go-keyring` exposes `Set`, `Get`, `Delete` and `DeleteAll`, and **no way to enumerate**.
While a credential's name was owned by the code that needed it, that did not matter: the daemon could ask `Has` once per registered source. A user-chosen label cannot be guessed, so with §1 there is nothing to iterate over.

So a `credential_names` table stores `(source, label, created_at, updated_at)`, unique on the pair.
It is the answer to `GET /v1/credentials`, and it is deliberately named for what it holds.
There is no value column, and a migration that adds one is a bug, not a feature.

The alternative — keeping the index as one more keychain item — was rejected for the reason the "one shared blob" alternative below is rejected: the 3009-byte ceiling would become a limit on how many credentials may exist, which is a strange thing to discover at the point of adding one.

The table and the keychain can disagree, because they are two stores and nothing spans them atomically.
Writes go keychain first, then index, so a crash between them leaves a stored secret nothing lists — recoverable by setting it again.
The reverse order would leave a listed credential that no connector can read, which looks like corruption to the user and reads as configured to the UI.
`Get` on a ref the keychain does not have returns not-found whatever the index says: the keychain is the truth, and the index is a directory.

### 7. Nothing is logged, and nothing is returned in an error

A credential's *ref* may be logged. Its value may not, and neither may any prefix of it.
Errors from the store are wrapped with the ref only.

This needs a test, not a convention, because the natural way to write the failure path — `fmt.Errorf("storing %q: %w", secret, err)` — is one character away from correct.

## Consequences

- Google Programmable Search becomes buildable, as do Adzuna, USAJOBS and Jooble. None is built by this ADR.
- A new dependency, and a first use of an OS facility the product has not touched.
- On Linux, `Set` will fail where no Secret Service is running. This is measured on macOS only; the Linux and Windows paths are the library's, and are **not verified here**. Whoever implements this should measure both before claiming they work.
- The desktop gains a settings surface for entering a key and seeing that one is set, and cannot show it again afterwards. That is a deliberate, visible limitation and the UI should say so rather than appear to have lost it.
- A migration, for `credential_names`. It stores no secret, and exists only because the keychain cannot be enumerated.
- Two stores that can disagree, with a recovery path rather than a transaction. See §6.
- A source may now hold several credentials, and nothing yet says which one a run uses. See the open question below.

## Alternatives considered and rejected

**Storing the key in the browser profile**, the way board logins are handled. It works for a site a human logs into, and not at all for an API key sent as a query parameter by a Go connector.

**Asking the user for the key on each run.** Acceptable for a CLI, unacceptable for a Scout, which is by definition unattended.

**One shared "secrets" blob.** A single keychain item holding JSON for every credential. Fewer items, but every read decrypts every secret, and the 3009-byte ceiling becomes a limit on how many credentials exist rather than on how long one is.

## Settled when this was read

**The API is write-only.** §4. No route returns a secret, and the one that lists them returns refs only.

**A credential is scoped per source and label.** §1. This was proposed as a deferral and decided the other way, which is why §6 exists: user-chosen labels cannot be enumerated from a keychain that has no list operation, so the product must keep a directory of its own.

## Open questions

1. **Does the Linux path work on a plain GNOME desktop, and what exactly happens without a Secret Service?** The fallback in §5 assumes it fails cleanly and distinguishably. If it hangs on a D-Bus call instead, the override is not enough and a timeout is needed. Measured on macOS only; Linux and Windows are unverified.

2. **Which label does a run use, when a source has several?** Identity is settled; selection is not. Nothing in the product configures a job source per-run today — a saved search names criteria, not connectors — so there is no field to put a choice in and no honest way to invent one here. Until a source gains configuration, a connector asks for `default`. The moment two labels can both be live, this needs deciding, and it is a product question rather than a storage one.
