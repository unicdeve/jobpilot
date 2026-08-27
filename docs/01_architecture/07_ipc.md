# IPC Architecture

## Requirements

- Local only by default.
- Cross-platform.
- Supports desktop and CLI clients.
- Does not require a cloud service.
- Supports structured requests and events.

## Initial direction

Use a local API/IPC boundary between UI/CLI and daemon. The exact transport should be chosen during Phase 0 based on Tauri integration, cross-platform behavior, and simplicity.

Candidates include:
- local HTTP over loopback
- Unix domain sockets where supported
- named pipes on Windows

The implementation should hide the transport behind a client interface.
