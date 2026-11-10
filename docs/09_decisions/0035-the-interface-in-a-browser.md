# ADR-0035: The interface in a browser

## Status

Proposed on 2026-11-10.
The user asked on 2026-11-10 for the graphical interface to be usable in a web browser, with the desktop app optional.
This ADR records how, for approval.

It extends ADR-0004 (Tauri and React): the desktop app stays, and the same React app is also served by the daemon.
It extends ADR-0012 (loopback HTTP) with a check on the `Host` and `Origin` of every request.
It narrows ADR-0028 (notifications): someone who uses only the browser gets no operating-system notifications.
It extends ADR-0029 §3: the standalone daemon and CLI carry the browser interface.

## Context

### The interface is already a web page

The desktop is a React app in a Tauri webview.
Every piece of data it shows comes from the daemon's loopback HTTP API through `fetch` (`desktop/src/lib/api.ts`).
The webview is used for very little else, all of it in the frontend:

- `ensure_daemon` and `find_daemon`, Rust commands that start or find the daemon and return its port, and a `daemon-waiting` event while an older daemon stops (ADR-0029 §4);
- `app_version`, the app's own version, shown beside the daemon's under Settings → About;
- the opener plugin, which opens a posting or a hiring page in the user's own browser;
- the dialog plugin, which picks a resume and returns its path, which the daemon then reads (`POST /v1/resumes` takes `file_path`);
- the window's theme, which is already skipped outside Tauri.

The router is a `HashRouter`, so every screen is the one `index.html`.

### Not everyone wants the desktop app

ADR-0029 §3 already ships the daemon and CLI alone, "for a machine without the desktop".
That machine has no graphical interface at all today.
Someone on a Linux distribution the packages do not cover, or who prefers to keep JobPilot in a browser tab, has to use the CLI.

### A browser is not a webview

A web page can be opened by anything that can reach the port, and any website the user visits can make requests to `127.0.0.1`.

ADR-0012 made loopback the boundary, and the CORS allowlist in `httpapi.withCORS` keeps another website from *reading* a response.
Two gaps exist today, before any page is served:

- **DNS rebinding.** A website can point its own name at `127.0.0.1` once its page has loaded.
  Its requests are then same-origin to the daemon, CORS does not apply, and it can read every response.
  The daemon never looks at the `Host` header, so it answers.
- **Cross-site requests.** A form on another website can `POST` to the daemon without a preflight.
  It cannot read the answer, but `POST /v1/shutdown` or a deletion does not need it to.

A daemon that serves a page makes both more likely to be tried, because a page at a known path says a JobPilot daemon is there.

### The daemon's port changes on every start

The daemon listens on a port the OS chooses (ADR-0012), written to the discovery file.
A bookmark to the interface stops working the next time the daemon starts.

### A browser cannot hand over a path

A file input gives a page the file's name and bytes, never its path.
The daemon cannot import a resume a browser picked through `POST /v1/resumes` as it is.

## Decision

### 1. The daemon serves the interface

The daemon serves the desktop's built frontend at `/`, from files compiled into its binary.
The API stays under `/v1/`.
The page and the API are the same origin, so the page needs no CORS and calls the API with relative URLs.

The files are compiled in only when the binary is built with the `webui` tag, after `desktop/dist` has been built and copied into `internal/adapters/primary/webui/dist`.
`make dist` and `make sidecars` do both, so every release has the interface; `make build-web` does it for a checkout.
A binary built without the tag, which is what `go build`, `go test` and `make build` produce, serves a short page at `/` saying the interface was not built in and how to build it.
That keeps every Go build and check from needing Bun.

### 2. `jobpilot-cli ui` opens it

`jobpilot-cli ui` finds or starts the daemon as every CLI command does, prints the interface's address, and opens it in the user's default browser.
`--no-open` only prints it.

The port is not fixed.
When the daemon has stopped, the open page says so and that `jobpilot-cli ui` opens it again; it cannot follow a new daemon to a new port as the desktop does.
A fixed port can be added later as a setting if this turns out to be a burden, and nothing here prevents it.

### 3. The daemon answers only to its own names

Every request is refused with `403` unless:

- its `Host` names the loopback address: `127.0.0.1`, `localhost` or `[::1]`, on any port; and
- it carries no `Origin`, or its `Origin` is one of the desktop's (as today), or it is `http://` followed by the request's own `Host`.

The first closes DNS rebinding, since a rebinding page's requests name the attacker's host.
The second refuses a request another website's page starts, whatever its method or content type.
The CLI and the desktop's Rust code send no `Origin` and name `127.0.0.1`, so they are unaffected.

No per-launch token is added.
Any process running as the user can already reach the API (ADR-0012), so a token would protect against nothing the two checks leave open.

### 4. A resume can be uploaded

`POST /v1/resumes/upload` takes the file as `multipart/form-data`, with `candidate_id` and the file under `file`, up to 20 MiB.
The daemon converts it as it would the same file on disk, through the same `ports.DocumentConverter`.
The resume records the file's name and no original path, which the browser never had.

The desktop and the CLI keep `POST /v1/resumes`, which reads a path, since they run on the machine that has the file.

### 5. One frontend, and the platform behind one module

`desktop/src/lib/platform.ts` is the only frontend module that knows whether it runs in Tauri.
It decides from `__TAURI_INTERNALS__`, which Tauri defines before the page's scripts run.

| What | In the desktop | In a browser |
| --- | --- | --- |
| Reach the daemon | `ensure_daemon`, `find_daemon` | The page's own origin |
| Open a link | Opener plugin | A new tab, with `noopener` and `noreferrer` |
| Pick a resume | Dialog plugin, a path | A file input, the file's bytes |
| The app's version | `app_version` | The daemon's, since it served the page |

Nothing else in the frontend imports a Tauri package.

### 6. No notifications without the desktop app

A scout's notification is posted by the desktop app, because a notification belongs to an app (ADR-0028 §1).
Without it installed, the daemon records the notification and posts nothing, as it already does when it has never been told where the app is.
What a scout found is still in the interface when the user opens it.

A browser's own notifications need the tab open, and a scheduled run usually happens when it is not.
They are not used.

## Consequences

- The standalone release has a graphical interface, and the desktop app is optional.
- The daemon's binary grows by the size of the built frontend, a few megabytes.
- Every request is checked for its `Host` and `Origin`, which closes two gaps that exist with or without the browser interface.
- A browser user has no notifications, and reopens the interface with `jobpilot-cli ui` after the daemon restarts.
- A release build needs Bun for the standalone archives too.
- In development, `bun run dev` serves the interface at `http://localhost:1420`, which proxies `/v1/` to whichever daemon the discovery file names, so it works in a browser as well as in `tauri dev`.

## Alternatives considered

- **Serve the files from a directory next to the binary.** Rejected: the archive and every installer would have to carry and find a directory, and a binary without it would serve nothing.
- **Commit the built frontend.** Rejected: a generated bundle in the history, and one that is wrong whenever the source changes without it.
- **A fixed port.** Deferred: it can collide with another program, and the CLI already knows how to find the daemon.
- **A per-launch token in the address.** Rejected for the reason in decision 3.
- **Upload in the desktop too, and drop the dialog plugin.** Rejected for now: the desktop's picker is known to work in every webview Tauri uses, and showing the path is useful there.
