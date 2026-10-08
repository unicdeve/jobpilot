# Daemon Architecture

## Responsibilities

- Coordinate local services
- Execute scheduled jobs
- Manage agent runs
- Expose local API/IPC
- Coordinate storage
- Coordinate LLM provider
- Coordinate discovery connectors
- Coordinate browser preparation
- Emit structured events
- Enforce policy boundaries

## Lifecycle

The daemon is not required to run permanently.

Supported lifecycle patterns:

1. CLI commands find or start the daemon:
   `cmd/jobpilot-cli` auto-starts `cmd/jobpilot` as a detached process if none
   is reachable (see `docs/09_decisions/0012-loopback-http-ipc.md`).
2. Desktop application starts/communicates with a local daemon when needed.
3. OS scheduler starts the daemon for scheduled discovery. Implemented for
   scheduled scouts (ADR-0027): each one's alarm in launchd, systemd or Task
   Scheduler runs `jobpilot scout wake <id>`, a client of the daemon that
   starts one if none is running, asks it to run the scout if it is due, and
   exits. The daemon it starts is detached, in a session of its own, so it
   outlives the alarm that started it.

There is no daemon or UI login item (ADR-0029 §9).
Scheduled Scout alarms provide startup for due work after a restart.

One daemon runs at a time.
It owns one isolated runtime per registered local workspace, including a SQLite connection, core services and browser profile for each (ADR-0032).
A daemon holds an exclusive lock on `daemon.lock` in the data directory for as long as its process runs, and refuses to start while another holds it, because two would share the workspace catalog and operating-system integrations, and clients would find only whichever wrote the discovery file last.
The operating system lets go of the lock when the process exits, however it exits, so a daemon that crashed leaves nothing behind that stops the next one.
A file left by a daemon that did not shut down cleanly names one that does not answer, and is written over.
A daemon shutting down removes the discovery file only if it still names that daemon.

A client that finds the lock held and no daemon answering is looking at one starting or stopping, and waits for it to answer or to go before starting another.

Every client compares its version with the daemon's, from `/v1/health` (ADR-0029 §4).
A newer client asks the daemon to stop with `POST /v1/shutdown`, waits for its process to let go of the lock, and starts its own; an older one uses the daemon, and the CLI says that it is older.
A version of `dev`, which every build but a release's has, matches any other.
`jobpilot stop` stops the running daemon the same way, and is what the Windows installer runs before replacing or removing it.
A daemon whose active workspace database has migrations it does not know refuses to start, since a newer JobPilot wrote it and no release migrates downward.
An inactive workspace that cannot be opened is left unavailable and retried when selected rather than preventing access to healthy workspaces.

The desktop follows a daemon that restarts: a request that cannot reach it makes the desktop wait for a daemon to answer, without starting one (`docs/08_development/hot_reload.md`).

The daemon should exit cleanly when there is no work and no reason to remain
alive. Auto-exit on idle is not yet implemented — today the daemon only
stops on an explicit signal (`SIGINT`/`SIGTERM`), or when a client asks
over `POST /v1/shutdown`, which it treats the same way.

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
- plugin processes if a future plugin runtime is implemented
