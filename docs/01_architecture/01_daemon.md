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

1. CLI command starts a short-lived process when appropriate.
2. Desktop application starts/communicates with a local daemon when needed.
3. OS scheduler starts the daemon for scheduled discovery.
4. Optional user-configured startup integration can launch the daemon/UI on login.

The daemon should exit cleanly when there is no work and no reason to remain alive.

## Concurrency

Use Go concurrency primitives carefully. Work should be cancellable using `context.Context`.

## Shutdown

Gracefully stop:
- scheduled work
- active network requests
- LLM calls where possible
- browser sessions
- database writes
- plugin processes
