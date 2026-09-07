# Desktop Architecture

## Stack

- Tauri 2
- React
- TypeScript
- Vite
- Tailwind CSS
- shadcn/ui or equivalent accessible component system

## UX goal

The application should feel close to a native desktop application rather than a website placed in a window.

## Responsibilities

The desktop app handles:
- dashboard
- jobs
- matches
- applications
- profile
- resume management
- settings
- prompt customization
- agent activity
- approval/review UI

The desktop app does not own business logic that must also work through the CLI.

## Communication

Use a local, authenticated-by-local-process-boundary IPC/API mechanism appropriate to the platform. Do not expose the daemon broadly on the network by default.

See `docs/01_architecture/07_ipc.md` and `docs/09_decisions/0012-loopback-http-ipc.md`:
the daemon exposes a loopback HTTP API (`internal/adapters/primary/httpapi`)
that the CLI already uses as a client. The desktop app is expected to talk to
the same API rather than duplicating service wiring.
