# ADR-0036: The interface on Exact

## Status

Proposed on 2026-11-15.
The user asked on 2026-11-15 to migrate the web and desktop interface to Exact (https://github.com/expo/exact).
This ADR records the first step and what is still undecided, for approval.

It would supersede ADR-0004 (Tauri and React) for the interface, and make ADR-0035's served React app a transition.
Nothing is superseded until this is accepted.
The Tauri app and the React interface are unchanged and remain the product.

## Context

The interface is a React app in a Tauri webview, about 14,000 lines across more than 40 screens and components.
Every piece of data it shows comes from the daemon's loopback HTTP API (ADR-0012).
Exact writes an app once in Contract (a declarative view language) with data logic in TypeScript or Rust, and runs it on the web, macOS, iOS, and Linux.
Web uses the browser's DOM; macOS and iOS use AppKit and UIKit, with no webview.

Exact is pre-1.0, has no releases or API stability, and is consumed by path from a checkout.
The first step therefore has to be a slice that can be abandoned, not a rewrite.

## Decision

### 1. A vertical slice lives beside the existing interface

`exact-ui/` is an Exact app that reads the daemon's `/v1` API: a Jobs list with search, a job's detail with its match, and an Applications list.
It touches no external application form and writes no application content (ADR-0034).
It reads through `net.fetch` to one address, `API_BASE_URL`.

### 2. Tests run against a stand-in for the daemon

`exact-ui/server.ts` answers the read routes the app uses with fixed data.
`app.test.contract` runs unchanged on web and macOS.
The stand-in is not the daemon: contract drift between the two is not caught until a test runs against the real daemon.

## Open questions

- **Origin.** ADR-0035 §3 refuses a request whose `Origin` is not the desktop's or the daemon's own.
  A native Exact host sends no `Origin` and is unaffected.
  An Exact web build served from its own port is refused, so the web build must be served by the daemon, as ADR-0035 serves the React build.
- **Finding the daemon.** The daemon's port changes on every start.
  The slice takes it from `API_BASE_URL`; reading the discovery file needs a grant and a decision.
- **Tauri's other jobs.** Starting the daemon, notifications (ADR-0028), the resume dialog, and the opener plugin have no Exact counterpart yet.
- **Toolchain.** Building the Apple hosts needs Xcode 27.1 or later, and a release needs a path through Exact's checkout, which has no versions.
- **Scope.** Profile, resumes, scouts, activity, settings, and the application detail are not migrated.

## Consequences

- Nothing in the shipped product changes.
- The slice shows what the interface looks like in native controls on macOS and iOS.
- Each open question needs an answer before this ADR is accepted.

## Alternatives considered

- **Replace the React interface in place.** Rejected: Exact has no API stability, and the interface has no complete replacement.
- **Stay on Tauri and React.** The status quo, and what remains if this is not accepted.
