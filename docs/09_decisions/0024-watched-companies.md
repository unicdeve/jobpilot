# ADR-0024: Watched companies

## Status

Accepted, and implemented.
Accepted on 2026-10-01, after watching companies on a real installation.
It records `docs/02_features/17_company_watchlists.md` D4 (decided on 2026-09-30), and how the feature's other decisions are built.
It changes one thing the spec said, from measurement: see §6.

## Context

`docs/02_features/17_company_watchlists.md` lets the candidate name companies and finds their openings on the companies' own hiring pages, hosted by Greenhouse, Lever or Ashby.
D1 to D6 settle what the feature does.
What is left is where the list lives, how a company's page becomes jobs, and how discovery reports a hundred companies as one source.

Measured on 2026-09-30, against the three providers' public endpoints:

- Each answers a board that does not exist with 404: Greenhouse `{"status":404,"error":"Job not found"}`, Lever `{"ok":false,"error":"Document not found"}`, Ashby `Not Found`.
- Greenhouse's board endpoint names the company (`{"name":"GitLab"}`).
  Lever's and Ashby's APIs do not; their hosted pages do, Lever's in its `<title>` and Ashby's in the organization its page embeds (`"name":"ClickUp"`).
- A board's name is not a company's.
  Of 120 names probed, Greenhouse's `wise` is "Wise Worksite Field Sales" and its `remote` is "General Assembly Remote Jobs"; Ashby answers several names with an empty board.
- Every location a posting lists is free text, and ADR-0023's reading already reads it: "Remote (United States | Canada)", "Home based - EMEA", "UNITED STATES - Remote, EMEA - Remote", "Remote France; Remote Germany; Remote UK".
  The one word it missed was "AMER", which ClickHouse, Supabase, Fivetran and Remote use for the Americas.

## Decision

### 1. A board is named by its provider, its region and its name

```go
type Board struct {
    Provider BoardProvider // greenhouse | lever | ashby
    Region   string        // "" or "eu"
    Name     string        // Greenhouse's token, Lever's site, Ashby's job board name
}
```

`domain.ParseBoard` reads a board from an address, deterministically and without a request:

| Provider | Addresses |
| --- | --- |
| Greenhouse | `boards.greenhouse.io/{token}`, `job-boards.greenhouse.io/{token}`, their `.eu.` forms, and `…/embed/job_board?for={token}` and `…/embed/job_app?for={token}` |
| Lever | `jobs.lever.co/{site}`, `jobs.eu.lever.co/{site}` |
| Ashby | `jobs.ashbyhq.com/{name}` |

A posting's address names its board the same way, since the board is the first segment of every path above.
Any other host is refused with this table, and nothing is fetched.
Two boards are the same board when provider and region are equal and their names are equal ignoring case.

A job offers "Watch this company" when its canonical URL, or the first address on a supported board in its description, names a board (`domain.BoardOf`).
The description is where a Hacker News posting links to the company's page (migration `00013`).

### 2. One table, the candidate's own

```sql
CREATE TABLE watched_companies (
    id           TEXT PRIMARY KEY,
    provider     TEXT NOT NULL,
    region       TEXT NOT NULL DEFAULT '',
    board        TEXT NOT NULL,
    name         TEXT NOT NULL,
    added_from   TEXT NOT NULL,
    added_at     DATETIME NOT NULL,
    checked_at   DATETIME,
    check_status TEXT NOT NULL DEFAULT '',
    open_roles   INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX idx_watched_companies_board
    ON watched_companies(provider, region, board COLLATE NOCASE);
```

`check_status` is how the last check went: `ok`, `gone` (the provider answered 404), `unreachable` or `failed`, and empty before the first.
`open_roles` is how many postings the board held at that check, before any search's criteria, so the list can say "40 open roles" of a company whose roles no search wanted.
It is the one column D4 did not list.

The table references nothing and nothing references it, so ADR-0021 §1 gains no rule.
It gains a record kind, `watched_company`, which "delete all data" deletes and deleting the profile does not.

### 3. Watched companies are one source, built in core

`services.CompanyWatchService` holds the list and provides a `ports.JobSource`, "Company pages" (`company-pages`), which discovery asks alongside the job boards.

Its `Search` lists the watched companies, reads each board at most four at a time, and returns every posting; `FiltersServerSide` is false, so discovery applies the criteria as it does for every source (D3).
Each read is recorded on its company: `checked_at`, `check_status` and `open_roles`.
A job with no company name of its own is given the watched company's.

It is in core rather than in the discovery adapter because it reads the candidate's data and writes to it.
The adapter reads boards and knows nothing of who watches them.

### 4. Reading a board is a port

```go
type BoardReader interface {
    ReadBoard(ctx context.Context, board domain.Board) (*domain.BoardContents, error)
    BoardName(ctx context.Context, board domain.Board) (string, error)
}
```

`discovery.CompanyBoards` implements it for the three providers.
`ReadBoard` returns the board's postings as jobs and when they were fetched.
A board that answers 404 is `ports.ErrBoardGone`; no answer at all wraps `ports.ErrSourceUnreachable`, as every connector does.

Each board's answer is cached for an hour (D5), and concurrent reads of one board share a single request, since a Scout and a launch refresh can ask at once.
A failure is not cached.
`checked_at` is the answer's fetch time, so a cached answer does not claim a new check.

`BoardName` is used once, when a company is watched.
Greenhouse's comes from its board endpoint and Lever's and Ashby's from their hosted pages; when none is found, the name is the board's, and the candidate can rename it.
Either way it is text from outside, stored and shown as plain text, at most 100 characters.

### 5. A source made of parts says which part failed

`domain.SourceOutcome` gains `failing`, the names of the parts of a source that did not answer.
A source returns `*ports.PartialAnswer` alongside its jobs when some part failed:

```go
type PartialAnswer struct {
    Failing []string // the parts' names, e.g. "GitLab"
    Err     error    // nil when some part answered
}
```

Discovery takes the jobs, reports the source as answered with its failing parts named, and treats `Err`, when set, as the source's own failure: unreachable when every part was unreachable, failed otherwise.
So a company whose board has gone is named in what a search says, the other companies' jobs and the other sources' are kept (AC-005), and the company stays on the list, marked `gone`, until the candidate removes it.

### 6. Where a posting can be done from is read from its locations

The spec's principle 4 said a structured field — Lever's `country`, Ashby's `address` — would be used as it is.
Measured, neither says where a job can be done from; each names the posting's office or hiring entity:

- Railway's roles labelled "Global" carry a San Francisco address, and Oyster's "Any Location" one in Barcelona.
- Lever's `country` is GB for Toptal's "Europe and Canada" roles and SG for Binance's "Asia".

Used as places, they would narrow a worldwide role to one country.
So each provider's own location list is what ADR-0023 reads: Greenhouse's `location.name`, split at `;`; Lever's `categories.allLocations`; Ashby's `location` and each of `secondaryLocations`.
The workplace type is the structured field that is used: Lever's `workplaceType` and Ashby's (or `isRemote`) become `RemoteType`, and Greenhouse's, which has none, is read from its location's words "remote" and "hybrid".
Salary is taken where it is structured: Lever's `salaryRange` and Ashby's `compensation`.

Reading these boards also found three gaps in ADR-0023's reading, fixed with this decision:

- "AMER", which ClickHouse, Supabase, Fivetran and Remote write, is the Americas.
- A country's name that is also a state's — "Georgia" — is the state when the rest of the location is in the state's country: "Remote, US, Georgia" and "Atlanta, Georgia" are in the United States, "Tbilisi, Georgia" in Georgia.
- A three-letter name is a city only when it is the city's own, like Ulm or Van. GeoNames lists airport codes among cities' alternate names, which read Stripe's "Tor", for Toronto, as Slovyansk; "NYC" and "SF" are kept by hand.

`domain.SignalsVersion` becomes 2, so every stored job is read again with them (ADR-0023 §3).

### 7. The starter list is generated

`internal/tools/genstarter` reads `seed.txt` — one address per line, nothing else — reads each board with `discovery.CompanyBoards`, and reads each posting with `domain.ReadSignals`, so the measurement is exactly what the product would read.
A board is kept when at least one posting can be done from anywhere, or is listed for countries on more than one continent, by GeoNames' continents (ADR-0023 §2), and when its board names the company, since a suggestion must say whose it is.
The list is ordered by how many postings qualified, most first.
`-explain` prints each qualifying posting and how it was read, so a reviewer can see what put a company on the list.

It writes `internal/core/domain/starter_companies_gen.go`: for each kept board, its address, the company's name as `BoardName` gives it, the date, the open postings and how many qualified.
`make starter-list` runs it, before each release.
A board that did not answer, or did not qualify, is written to the command's output and not to the list.

### 8. Surfaces

| Route | Does |
| --- | --- |
| `GET /v1/companies` | the watched companies |
| `POST /v1/companies` | watch `{address}` or `{job_id}`; 201, or 200 with the company already watched |
| `PUT /v1/companies/{id}` | rename, `{name}` |
| `DELETE /v1/companies/{id}` | stop watching; the company's jobs stay |
| `GET /v1/companies/suggestions` | the starter list, less what is watched |
| `GET /v1/jobs/{id}/company` | the job's board, and the watched company it is, if any |

An unsupported address is 400, a board that answers 404 is 422, and no answer is 503.
The CLI gains `company watch <address>`, `company list`, `company rename <id> <name>`, `company unwatch <id>` and `company suggestions`, which lists each suggestion's address to watch.

## Consequences

- One new table, and a record kind in ADR-0021's deletion.
- Discovery gains a source whose size is the candidate's list; with none watched it answers at once with nothing.
- `SourceOutcome` gains a field that both clients read, and the desktop's summary of a search names a failing company.
- Stored jobs are read once more on the next start, for "AMER".
- The starter list is only as current as its last regeneration, and says when that was.

## Alternatives considered

**A source per company.**
Each would be named as it failed with no new field, but a hundred companies would be a hundred sources asked at once, past D5's four at a time, and a hundred lines in every search's summary.

**The watch list in the discovery adapter.**
It would need a repository inside an adapter, and writing a check's outcome back would make a job-source connector a writer of the candidate's data.

**Structured location fields over free text.**
Rejected by measurement (§6).
