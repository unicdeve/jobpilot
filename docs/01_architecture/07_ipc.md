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
