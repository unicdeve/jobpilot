# Plugin Architecture

## Direction

The core application should expose stable extension points for:

- job sources/connectors
- agent tools
- notifications
- future integrations

## Initial MVP

Do not build a public plugin marketplace.

Keep extension interfaces internal and design them so they can later be extracted into an SDK.

## Post-MVP protocol

Prefer process-isolated plugins communicating through a stable protocol such as JSON-RPC over stdio or another documented local IPC mechanism.

Do not use Go's native `plugin` package as the primary cross-platform extension mechanism.

## Security

Plugins are untrusted extensions. Future plugin permissions must explicitly define filesystem, network, browser, and application capabilities.
