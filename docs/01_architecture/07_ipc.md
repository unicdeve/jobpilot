# IPC Architecture

## Requirements

- Local only by default.
- Cross-platform.
- Supports desktop and CLI clients.
- Does not require a cloud service.
- Supports structured requests and events.

## Decision

Local HTTP over loopback (`127.0.0.1`, OS-assigned ephemeral port), with the
daemon publishing its port via a discovery file in the application data
directory. See `docs/09_decisions/0012-loopback-http-ipc.md` for the
rationale (chosen over Unix domain sockets/named pipes for cross-platform
simplicity with no new dependency) and consequences.

The transport is hidden behind a client interface:
`internal/adapters/primary/httpapi.Client`. The daemon's handler lives in the
same package.

## Reads and writes are separate endpoints

Discovery illustrates a rule the API follows generally: fetching from an
external source and reading local history are different operations with
different failure modes, so they are different endpoints.

- `POST /v1/jobs/search` queries every registered job source, stores what it
  finds, and returns it. It needs internet access and it writes.
- `GET /v1/jobs` returns jobs already stored, paginated (`limit`, `offset`)
  and optionally filtered (`q`), in a `{"jobs": [...], "total": N}` envelope.
  It contacts no source and writes nothing, so it works offline.

Collapsing the two — using an unfiltered search as "list everything" — makes
browsing history impossible without a network round trip, which
`docs/02_features/04_job_discovery.md` AC-006 and the offline user flow both
forbid. `total` describes the whole filtered result rather than the page, so
a client can tell whether more pages remain without fetching them.
