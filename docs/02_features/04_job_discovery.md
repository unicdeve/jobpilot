# Feature: Job Discovery

## Purpose

Find jobs using configured criteria and supported sources.

## Search criteria

- Keywords
- Job title
- Location
- Remote preference
- Employment type
- Salary preference
- Experience preference
- Included sources
- Excluded companies

## Where criteria come from

Discovery runs only against a saved search the user chose.
Writing a good one requires knowing how `SearchCriteria.Matches` behaves, which the form does not show, so the product also ships measured criteria the user can adopt and edit.
See `docs/02_features/14_search_templates.md`.

## Source strategy

Discovery is connector-driven, not scraper-first.

Initial public source implementations should prioritize stable APIs/feeds/public ATS pages where practical.

Five sources are registered, all public APIs needing no credentials, so discovery works on a fresh install with nothing configured:

| source | what it adds |
| --- | --- |
| Arbeitnow | European board, many German listings |
| Remotive | remote roles, tagged by category |
| RemoteOK | remote roles, a large single feed |
| Himalayas | remote roles, and the only source that reliably reports a salary range with its currency and period |
| Hacker News, "Who is hiring?" | postings that exist on no job board, written by the hiring team |

### Why Google is not a source

Searching Google by hand finds jobs no board lists, and it was asked for on those grounds.
It is not available to this product, for a reason that is practical before it is legal.

Measured: a request to `google.com/search` returns HTTP 200 and a JavaScript gate — **zero result links**, no job URLs of any kind.
Getting results would mean driving the browser worker through that gate, and ADR-0013 and ADR-0014 both state that JobPilot never defeats an anti-bot measure.
Google's terms also prohibit automated querying.

The legitimate route is Google's Programmable Search JSON API, which is official and free up to 100 queries a day.
It is not built because it needs an API key, and a stored credential needs OS-native secure storage, which the product does not yet have.
That is a decision with its own cost, not a connector.

The Hacker News source exists partly to serve the same need: those postings are written by the people hiring and appear on no board.

### Reading a source that has a convention instead of a schema

Hacker News postings follow a convention — `Company | Role | Location | Type | Salary | URL` — that nothing enforces.
Measured on the September 2026 thread, 92% of 257 top-level comments carry at least two pipes and 80% carry both pipes and a link.

A comment that does not fit is skipped rather than guessed at, and so is one with no link: a job nobody can apply to is a comment, and storing it would put an unusable row in front of the user and hand the browser worker a discussion thread to fill in.
Within a posting, an ambiguous field is dropped rather than assigned. A wrong location silently hides a job from the user's filter; a missing one leaves it visible.
Salary is deliberately not parsed from these postings: the formats vary too much to read without occasionally inventing a figure, and the raw text is kept in the description.

## Acceptance Criteria

### AC-001
A user can save search criteria.

Implemented as named saved searches (`saved_searches`, `SavedSearchService`,
`jobpilot-cli search saved`, and the desktop's Settings → Job searches). A
user keeps several and picks between them; one is the default that
`search run` uses when none is named.

Discovery refuses to run on empty criteria. Neither public source filters
server-side, so an unrestricted search would import every posting on every
board — roughly 266 jobs at the time of writing — rather than the jobs the
user wants. An installation with no saved search therefore cannot discover
jobs at all, which is why configuring one is part of onboarding
(`docs/02_features/01_onboarding.md`).

Criteria are keywords, title, location, remote type, employment type,
salary bounds and excluded companies. Keywords and title match every term in
any order, so "backend engineer" finds "Engineer, Backend (Go)"; a substring
test did not. Salary bounds keep jobs that advertise no salary, since most
boards publish none and treating absence as a mismatch would hide them.
Years-of-experience bounds and a source allowlist were removed: no connector
could populate them, and the API had been accepting and silently ignoring
them.

### AC-002
A search can be executed from the CLI.

### AC-003
A search can be executed from the desktop application.

### AC-004
Jobs from supported sources are normalized into the common Job schema.

### AC-005
Duplicate jobs are not created repeatedly for the same canonical job.

### AC-006
When internet access is unavailable, discovery fails clearly without corrupting existing history.

Already-discovered jobs stay readable in that state: `GET /v1/jobs` (the
desktop Jobs screen and `jobpilot-cli search list`) reads stored history
only and contacts no source. Searching for *new* jobs is the operation that
requires internet and reports clearly when it is unavailable.

### AC-007
Custom job URLs can be submitted for individual job analysis.

When no registered source's API can fetch a URL directly (true of Arbeitnow
and Remotive today), the URL is opened in a real browser (the same
Playwright worker used for application preparation) and its rendered page
is read, since many job postings are JS-rendered pages a plain HTTP fetch
wouldn't return usable content for. The LLM extracts structured fields
(title, company, location, remote/employment type, salary if stated,
skills, and the actual posting content) from the page — the page is
treated as untrusted external content, and a page the LLM can't identify a
job title on fails clearly rather than saving a placeholder job record.

### AC-008
Source failures do not prevent successful results from other sources.
