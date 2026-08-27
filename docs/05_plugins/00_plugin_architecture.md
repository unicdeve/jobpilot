# Plugin Architecture

## Status

Post-MVP.

## Goal

Allow community-developed extensions without requiring changes to the core runtime.

## Extension types

- Job source connector
- Agent tool
- Notification provider
- Future integrations

## Direction

Use process-isolated plugins with a stable protocol. JSON-RPC over stdio is a strong initial candidate because it is cross-platform and language-neutral.

The protocol should be versioned and capability-based.
