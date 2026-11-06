# Architecture Overview

## Implemented runtime

```text
Desktop (Tauri + React)     CLI (Go)
          |                   |
          +-- loopback HTTP ---+
                     |
                 Go daemon
                     |
        +------------+-------------+
        |            |             |
   Core services  LLM providers  Discovery connectors
        |
        +-- SQLite and OS credential storage
        +-- Go Playwright / installed Chrome
        +-- Scout alarms / OS scheduler
        +-- Desktop notification delivery
```

The daemon owns local state and coordinates the core services.
The desktop and CLI are clients of its loopback HTTP API (ADR-0012).
Core business rules remain independent of UI, storage and provider adapters.

Scouts discover, match and create applications for the candidate without opening the browser.
The candidate writes and sends their own applications; JobPilot never writes application content or touches an employer's form (ADR-0034).
The browser only reads a job page for import by URL.
A general model-directed tool runtime and external plugin loading remain proposals, not implemented subsystems.

## Lifecycle and resources

Clients and scheduled Scout alarms find or start a single daemon on demand.
Closing the desktop does not stop it; it remains running until shutdown is requested or it receives a termination signal.
Automatic idle shutdown is not implemented, and there is no daemon login item (ADR-0029).

Idle work should be event-driven and consume minimal resources.
See [Daemon architecture](01_daemon.md) for discovery, version negotiation, scheduling and shutdown.
