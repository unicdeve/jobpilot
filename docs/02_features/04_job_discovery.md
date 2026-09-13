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

## Source strategy

Discovery is connector-driven, not scraper-first.

Initial public source implementations should prioritize stable APIs/feeds/public ATS pages where practical.

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
