# Daemon Architecture

## Responsibilities

- Coordinate local services
- Execute scheduled jobs
- Manage agent runs
- Expose local API/IPC
- Coordinate storage
- Coordinate LLM provider
- Coordinate discovery connectors
- Coordinate browser automation post-MVP
- Emit structured events
- Enforce policy boundaries

## Lifecycle

The daemon is not required to run permanently.

Supported lifecycle patterns:

1. CLI command starts a short-lived process when appropriate. Implemented:
   `cmd/jobpilot-cli` auto-starts `cmd/jobpilot` as a detached process if none
   is reachable (see `docs/09_decisions/0012-loopback-http-ipc.md`).
2. Desktop application starts/communicates with a local daemon when needed.
3. OS scheduler starts the daemon for scheduled discovery. Implemented for
   scheduled scouts (ADR-0027): each one's alarm in launchd, systemd or Task
   Scheduler runs `jobpilot scout wake <id>`, a client of the daemon that
   starts one if none is running, asks it to run the scout if it is due, and
   exits. The daemon it starts is detached, in a session of its own, so it
   outlives the alarm that started it.
4. Optional user-configured startup integration can launch the daemon/UI on login.

One daemon runs at a time.
A daemon refuses to start while the discovery file names another that answers, because two would share one database and one browser profile, and the clients would find only whichever wrote the file last.
A file left by a daemon that did not shut down cleanly names one that does not answer, and is written over.
A daemon shutting down removes the discovery file only if it still names that daemon.
Two daemons starting at the same instant can both pass the check; nothing starts daemons that way today.

The desktop follows a daemon that restarts: a request that cannot reach it makes the desktop wait for a daemon to answer, without starting one (`docs/08_development/hot_reload.md`).

The daemon should exit cleanly when there is no work and no reason to remain
alive. Auto-exit on idle is not yet implemented — today the daemon only
stops on an explicit signal (`SIGINT`/`SIGTERM`).

## Concurrency

Use Go concurrency primitives carefully. Work should be cancellable using `context.Context`.

## Shutdown

Gracefully stop:
- scheduled work: a scheduled scout run belongs to the daemon rather than to
  the request that woke it, so shutdown cancels it and waits for it to
  record itself as failed before the database closes
- active network requests
- LLM calls where possible
- browser sessions
- database writes
- plugin processes
