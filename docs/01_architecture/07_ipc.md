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

## Reads and writes are separate endpoints

Discovery illustrates a rule the API follows generally: fetching from an
external source and reading local history are different operations with
different failure modes, so they are different endpoints.

- `POST /v1/jobs/search` queries every registered job source, stores what it
  finds, and returns it in a `{"jobs": [...], "new": N, "sources": [...]}`
  envelope, where `sources` says how each source answered (`ok`,
  `unreachable` or `failed`) and `new` counts the jobs not stored before. It
  needs internet access and it writes.
- `GET /v1/jobs` returns jobs already stored, paginated (`limit`, `offset`)
  and optionally filtered (`q`), in a `{"jobs": [...], "total": N}` envelope.
  It contacts no source and writes nothing, so it works offline.
- `GET /v1/jobs/{id}` returns one stored job, on the same terms.
- `GET /v1/sources` names the sources a search asks (`[{"id", "name"}]`), so
  a client can say what a search is waiting on while it runs.

`POST /v1/jobs/search` rejects empty criteria with 400 rather than importing
every posting both boards return.
A search that some sources did not answer is still a 200, naming them.
One that no source answered is 503 when none could be reached, which is what
being offline looks like, and 502 when at least one was reached and failed. Saved searches live under `/v1/searches`
(list, create, get, update, delete, and `POST /v1/searches/{id}/default`),
and `POST /v1/searches/{id}/run` discovers using a saved search's criteria.
That last one exists so no client has to read a search, hold its criteria and
post them back — the shape that let an empty search be submitted at all.

Collapsing the two — using an unfiltered search as "list everything" — makes
browsing history impossible without a network round trip, which
`docs/02_features/04_job_discovery.md` AC-006 and the offline user flow both
forbid. `total` describes the whole filtered result rather than the page, so
a client can tell whether more pages remain without fetching them.

## Deletion is previewed, then confirmed

A deletion is two requests (ADR-0021 §4).
`GET <resource>/deletion` returns what deleting the resource would take with it, by kind and count, and a token that identifies exactly that.
`DELETE <resource>?token=<token>` carries it out, provided what it reaches is still what the token names.

A `DELETE` without a token is 400: nothing is deleted that no one was shown.
One whose reach changed since — a scout created an application for the job meanwhile — is 409, with the reach as it is now in the body, so the client can show it and ask again without another request.
A record that does not exist is 404, and a deletion the rules refuse, such as the cover letter an application uses, is 409 with the reason.

The resources are `/v1/jobs/{id}`, `/v1/applications/{id}`, `/v1/cover-letters/{id}`, `/v1/resumes/{id}`, `/v1/profile`, and three that name no record: `/v1/data/activity`, `/v1/data/jobs` and `/v1/data`, which is everything.

"Not interested" is not a deletion and takes no token: `PUT /v1/jobs/{id}/interest` with `{"not_interested": true}` or `false`.
`GET /v1/jobs` leaves the jobs so marked out unless given `interest=not_interested` or `interest=all`, and `GET /v1/matches` leaves their matches out; `GET /v1/jobs/{id}/match` returns a job's latest match whether or not it is marked, or 404 if it has none.

## Checking a dependency before using it

`GET /v1/settings/llm/health` reports `{"configured": bool, "ok": bool,
"error": "..."}` for the stored LLM configuration. The two flags are
separate because the failures need different advice: nothing has been set up
yet, versus a backend that is set up but not answering.

It exists so a client can check before starting multi-step work that depends
on the LLM, rather than discovering the problem partway through and leaving
the user with a half-finished result. The probe is bounded well under the
providers' own 30s request timeout, since it runs in front of a UI action.

The daemon builds its provider from the stored settings on each request
(`llm.ReloadingProvider`), so this endpoint and the work it guards always
describe the same backend, and settings saved through `PUT /v1/settings/llm`
apply without restarting the daemon.
