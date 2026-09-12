# Desktop Architecture

## Stack

- Tauri 2
- React
- TypeScript
- Vite
- Tailwind CSS
- shadcn/ui or equivalent accessible component system
- TanStack Query for server state

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

## Navigation

Screens are reached through the objects they are about, not by carrying
identifiers between them. `docs/00_product/03_user_flows.md` describes the
flows job-first — "user opens job → loads candidate context → match score
persisted", "user selects job → system generates cover letter" — and the job
detail route (`/jobs/:id`) is where that happens: it shows the job, its match
if it has one, its applications, and the actions that create either.

The lists feed it rather than competing with it. A job on the Jobs list, a
match, and an application all link to the job they concern, and a match or an
application shows the job's title rather than its `job_id`. Nothing in the UI
asks the user to type or paste an identifier.

Matches and applications store only a `job_id`, so those screens fetch the
jobs they reference one at a time (`useJobsByID`) rather than through a bulk
endpoint. These are local SQLite reads over loopback, bounded by how many
jobs the user has personally analysed or applied to, and each result is
cached under the key the job detail screen reads, so opening one of those
jobs afterwards costs no request at all. A bulk read on the daemon is the
answer if those lists ever grow enough for this to matter.

## Server state

Screens do not call the daemon client directly. `desktop/src/lib/queries.ts`
wraps it in TanStack Query hooks, and `desktop/src/lib/api.ts` remains the
single typed description of the daemon's HTTP surface underneath.

What that buys, beyond avoiding a hand-written `useEffect` per screen:

- **Deduplication.** Identical requests in flight at the same moment become
  one. React's StrictMode deliberately double-invokes effects in
  development, which previously meant two concurrent job searches on the
  dashboard's first render — two writers racing on SQLite.
- **A cache across navigation.** Leaving a screen and returning no longer
  refetches, which is what lets a list keep its scroll position.
- **Declared invalidation.** A mutation names the queries it affects, so
  panels no longer reload everything after every action. Invalidation
  belongs on settle rather than on success wherever a *failed* call still
  changed server state — `usePrepareInBrowser` is the example: a blocked
  browser run moves the application to FAILED and records why.

Cached data belongs to one daemon, so reconnecting clears the cache rather
than showing a previous daemon's data.

## Scrolling

The window never scrolls. The sidebar is fixed and the `<main>` region beside
it is the app's only scrolling element, which rules out React Router's
`ScrollRestoration` twice over: it requires a data router (this app uses
`<HashRouter>` with `<Routes>`), and it restores the window's position rather
than an inner element's. `desktop/src/lib/use-scroll-restoration.ts` records
each route's position and puts it back, and the app shell owns it because the
app shell owns the scrolling element.

Restoring is not a single assignment. A screen's content arrives after the
effect runs — a cached query renders at once, but a loading one, fonts and
images all change the height afterwards — so the hook retries via a
`ResizeObserver` until the content is tall enough for the saved position to be
reachable, and gives up the moment the user scrolls themselves rather than
yanking them away from where they chose to be.

This depends on the query cache: a list that refetched from empty on every
visit would have nothing to scroll back into.

## Communication

Use a local, authenticated-by-local-process-boundary IPC/API mechanism appropriate to the platform. Do not expose the daemon broadly on the network by default.

See `docs/01_architecture/07_ipc.md` and `docs/09_decisions/0012-loopback-http-ipc.md`:
the daemon exposes a loopback HTTP API (`internal/adapters/primary/httpapi`)
that the CLI already uses as a client. The desktop app is expected to talk to
the same API rather than duplicating service wiring.
