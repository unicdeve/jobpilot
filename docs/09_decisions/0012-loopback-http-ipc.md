# ADR-0012: Loopback HTTP as the IPC Transport

## Status

Accepted

## Decision

The daemon exposes its API to local clients (the CLI, eventually the desktop
app) as JSON over HTTP bound to `127.0.0.1` on an OS-assigned ephemeral port.
The daemon writes its PID and port to a discovery file in the application
data directory on startup and removes it on clean shutdown; clients read
that file and confirm the daemon is actually reachable with a health check
before trusting it, since an unclean shutdown can leave a stale file behind.

The transport is hidden behind `internal/adapters/primary/httpapi.Client`, so
callers do not need to know they are talking to a separate process, per
`docs/01_architecture/07_ipc.md`.

The CLI starts the daemon itself (as a detached background process) if none
is reachable when it runs, per the "CLI command starts a short-lived process
when appropriate" lifecycle pattern in `docs/01_architecture/01_daemon.md`
and ADR-0009. Auto-exit of an idle daemon is not yet implemented.

## Rationale

`docs/01_architecture/07_ipc.md` left the transport as a set of candidates
(local HTTP over loopback, Unix domain sockets, named pipes on Windows) to be
decided during implementation. Loopback HTTP was chosen because it is a
single code path across macOS, Linux, and Windows using only the standard
library (`net/http`), whereas Unix sockets and Windows named pipes require
separate platform-specific implementations behind a shared interface. This
matches `AGENTS.md`'s "keep the core simple and modular" principle and adds
no new dependency.

Loopback binding alone is the security boundary for MVP: only local
processes can reach the port, consistent with `docs/01_architecture/02_desktop.md`'s
"local, authenticated-by-local-process-boundary IPC" and `08_security.md`'s
"no application account/authentication system." A per-instance auth token in
the discovery file was considered for defense in depth on shared machines,
but deferred as unjustified complexity until a concrete requirement exists.

## Consequences

- `cmd/jobpilot-cli` no longer opens the database or wires services
  in-process; it is a client of `cmd/jobpilot` like the future desktop app
  will be.
- Every core service the CLI needs must be exposed as an endpoint in
  `internal/adapters/primary/httpapi`, which is now the seam new CLI/desktop
  functionality is wired through.
