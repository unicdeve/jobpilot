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

## Look and feel

The goal above is met by a handful of rules, and a screen that breaks one is what makes the app read as a website again.

**Colour is a token, never a literal.**
`desktop/src/index.css` defines every colour for a light theme (`:root`) and a dark one (`.dark`), and screens use the tokens — `text-muted-foreground`, `bg-card`, `border-border` — never `text-slate-500`.
A literal looks right in one theme and wrong in the other, and before this rule 21 files held 276 of them, which is why the app had no dark theme even though the dark tokens existed.
Status colours are roles (`success`, `warning`, `info`, `highlight`, `destructive`), used as a tint behind text of the same tone: `bg-success/12 text-success`.
`StatusBadge` maps every state the daemon reports to a tone and a readable label; a raw value such as `READY_FOR_REVIEW` is never shown.

**The theme is the user's, and follows the OS by default.**
System, Light or Dark, under Settings → Appearance.
The choice is kept in the desktop's own storage rather than the daemon, because how this window looks is a preference of this device and the CLI has no use for it.
It is applied before React renders, so the first frame is already right, and it is passed to the native window so the title bar matches.

**The platform's font, and its conventions.**
San Francisco on macOS, Segoe UI on Windows, the desktop's own font on Linux.
Chrome is not text-selectable and buttons show the arrow cursor, as native controls do; text a person reads or copies opts back in with `select-text`.

**The window.**
It opens at 1280×820 and cannot be made smaller than 1000×640.
On macOS the title bar is drawn over the window's content (`src-tauri/tauri.macos.conf.json`), so the sidebar runs to the top edge and toolbars are drag regions; Windows and Linux keep their standard frame.

**Navigation.**
A grouped sidebar with icons, in the order of the ⌘1–⌘7 shortcuts (Ctrl on Windows and Linux), with the active local profile at the bottom.
Its menu switches or adds profiles and links to Settings, which remains available on ⌘,.
Its only badge is the number of applications waiting on the user, because it is the only count that asks for something.

**Lists beside their items.**
Jobs, Applications and Activity are split views (`components/split-view.tsx`): the list keeps its place and scroll while the selected item changes, and a window too narrow for both shows one, with a way back.
A list's filters live in the URL, so a link can open a filtered list and selecting an item keeps the list as it was.
The Jobs list keeps its filters behind a toolbar button (`lib/job-filters.ts`), which carries a dot while any are set, and the count above the list says when it is filtered.
A filter is applied by the daemon when the list is paged there (All jobs), and in the desktop when every item is already loaded (Analyzed).
Other screens are a page (`components/page.tsx`): a sticky toolbar with the title and actions, then titled sections.

**Two kinds of wait, told apart.**
A read from the daemon is quick and quiet: a small spinner where the content will appear (`Loading` in `components/status-line.tsx`).
Work JobPilot does for the user — the model writing a letter, weighing a match or reading a resume, a search across the job sources, a running scout — can take a minute on a local model, and is shown by an animated orb from `thinking-orbs` beside words saying what is happening (`Thinking` and `Orb` in `components/thinking.tsx`).
The orb is never spent on a plain load, so it always means "working for you", and its animation names the verb: composing, solving, searching or working.
The line sits where the result will land, not only on the button that asked for it, and a result being replaced stays readable until the new one arrives.
A cover letter being rewritten is also framed by a slow glow from `border-beam` (`components/thinking-frame.tsx`), the one place the beam is used.
Both follow the theme the user chose, and stand still under reduced motion.

**Few boxes.**
A section is a heading, not a card; a bordered `Panel` holds a grouped list or form, divided by hairlines.
Forms that are not the point of a screen — starting an application, adding a job by URL, editing a scout — open in a dialog.
Anything that cannot be undone asks first (`components/confirm.tsx`), naming what will happen on the button.

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

A match is not a screen of its own. It is a job with a score, so matches are
the Jobs list's "Analyzed" view, ranked by that score, and the analysis is
shown on the job.

Matches and applications store only a `job_id`, so those screens fetch the
jobs they reference one at a time (`useJobsByID`) rather than through a bulk
endpoint. These are local SQLite reads over loopback, bounded by how many
jobs the user has personally analysed or applied to, and each result is
cached under the key the job detail screen reads, so opening one of those
jobs afterwards costs no request at all. A bulk read on the daemon is the
answer if those lists ever grow enough for this to matter.

## Server state

Screens do not call the daemon client directly.
`desktop/src/lib/queries/` wraps it in TanStack Query hooks, one module per kind of record (`job.queries.ts`, `application.queries.ts`, `scout.queries.ts`, ...), and `desktop/src/lib/api.ts` remains the single typed description of the daemon's HTTP surface underneath.
Every cache key is declared once, in `queries/keys.ts`, so one module can invalidate another's data without restating its keys.
Screens import the hooks from the module that owns them; there is no barrel module.

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

Cached data belongs to one daemon, so reconnecting with Retry clears the cache
rather than showing a previous daemon's data.
A daemon that stops answering is different: the desktop waits for one to
answer again without starting one, and keeps the cache when it does, since a
daemon found through the discovery file serves the same data directory
(`docs/08_development/hot_reload.md`).

Cached and mounted screen state also belongs to one workspace.
Switching profiles clears the query cache and remounts the application shell under the new workspace identifier, so drafts, selections and scroll positions cannot cross the isolation boundary.
The theme provider sits outside that boundary because appearance is a device preference.

## Source layout

A screen small enough to read in one sitting is one module in `desktop/src/pages/`.
A larger one is a directory named after its route, whose `index.tsx` exports the screen and whose other modules each hold one section or dialog of it (`pages/application-detail/cover-letter-section.tsx`, `pages/settings/model-settings.tsx`).
Components shared by several screens live in `desktop/src/components/`, and logic with no markup in `desktop/src/lib/`.

## Rendering untrusted content

Job descriptions are stored as markdown and rendered by
`desktop/src/components/markdown.tsx`, never as raw HTML. That component is
where the threat model's "malicious webpage content" mitigations live for the
display path — no HTML passthrough, no image loading, http(s) links only and
opened externally. See `docs/07_security/00_threat_model.md`.

Anything else that displays external text should go through it rather than
reaching for a second renderer.

## Sections and navigation state

Each sidebar screen is a section (`desktop/src/sections.tsx`), and the app shell keeps a section alive while another is showing, as a tab in a native app is.
Leaving Jobs and coming back finds it as it was: the same job selected, the same search and filters, a half-typed search or dialog, and every scroll position inside it.
Nothing saves or restores any of that; it is never torn down.

Every React router unmounts a route that stops matching, so this is the shell's doing rather than the router's (`desktop/src/components/layout.tsx`):

- The top-level `<Routes>` sends every path but `/welcome` to the shell, which routes within sections itself.
- A section is rendered from its first visit on, inside React's `<Activity>`, which hides the inactive ones with `display: none`, keeps their DOM and state, and destroys their effects, so a hidden screen holds no subscriptions or timers.
- Each section has its own `<Routes location={…}>` with the location it was left at, so a hidden section keeps seeing its own path and query string rather than the window's.
- The sidebar links and the ⌘1–⌘7 shortcuts go back to where a section was left; from inside a section, its link goes to the section's list with nothing selected, keeping the list's search and filters.
  A link elsewhere in the app, such as a Dashboard figure, goes where it points.
- Dialogs, menus, popovers and selects portal into a container inside their section (`desktop/src/lib/portal-container.ts`), since a portal sits outside the DOM `<Activity>` hides; a dialog left open goes away with its section and is there again on return.
- Each section scrolls in its own box, and a split view's panes scroll on their own inside it.
  Both WKWebView and Chromium keep an element's scroll position across `display: none`, so no position is recorded.

The window never scrolls.
React Router's `ScrollRestoration`, which restores the window's position and needs a data router, has nothing to do here.

This depends on the query cache holding what a hidden section shows.
A hidden section's queries have no observers, so the cache keeps unobserved data for an hour (`gcTime`) rather than the default five minutes; a list that emptied while hidden would come back loading, at the top.

Each screen's module is loaded the first time it renders (`desktop/src/lib/lazy-page.ts`), and so is first-run setup, so the window starts by parsing the shell rather than every screen and the libraries only some of them use, such as the markdown renderer job descriptions need.
The shell loads every section's module a second after it appears (`preloadSections`), so a first visit does not wait on one.
While a module loads, its section shows nothing rather than a spinner, since it comes from disk and a spinner would only flash.

## Communication

Use a local, authenticated-by-local-process-boundary IPC/API mechanism appropriate to the platform. Do not expose the daemon broadly on the network by default.

See `docs/01_architecture/07_ipc.md` and `docs/09_decisions/0012-loopback-http-ipc.md`:
the daemon exposes a loopback HTTP API (`internal/adapters/primary/httpapi`)
that the CLI already uses as a client. The desktop app is expected to talk to
the same API rather than duplicating service wiring.
