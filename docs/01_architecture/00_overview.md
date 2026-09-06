# Architecture Overview

## High-level architecture

```text
Desktop UI (Tauri + React)
        |
        | local IPC/API
        v
Go Agent Daemon
        |
        +-- Scheduler
        +-- Agent Runtime
        +-- Policy Engine
        +-- Job Discovery
        +-- Application Domain
        +-- LLM Gateway
        +-- Plugin/Connector Runtime
        +-- Notifications
        +-- Storage
        +-- Secrets
        |
        +-- SQLite
        |
        +-- Go Playwright (post-MVP)
```

## Core design

The daemon is the system of record and long-running coordinator. It should not consume significant resources continuously when there is no scheduled or user-requested work.

The desktop application is a UI client of the daemon.

The CLI is another client of the daemon/core services.

## Runtime modes

- Interactive command
- Desktop session
- Scheduled/background execution

The daemon may be started on demand, launched by the desktop application, or invoked by OS scheduling mechanisms. It should not require a permanent 24/7 process.

## Resource principle

Idle JobPilot should consume minimal resources. Work should be event-driven, scheduled, or explicitly requested.
