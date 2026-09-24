# ADR-0020: Google Search is not a job source

## Status

Accepted.

Brave Search, the alternative below, is deferred rather than rejected, and will be revisited.

This corrects `docs/02_features/04_job_discovery.md` and ADR-0019, both of which named Google's Programmable Search JSON API as the legitimate route to Google results, blocked only on a place to store its key.
ADR-0019 built that place.
The route itself had closed in the meantime.

## Context

Searching Google by hand finds postings that no board lists, which is why it was asked for.

Scraping it was ruled out first.
Measured in September 2026, a request to `google.com/search` returns HTTP 200 and a JavaScript gate with zero result links.
Getting past that gate means driving the browser through an anti-bot measure, which ADR-0013 and ADR-0014 say JobPilot never does, and Google's terms prohibit automated querying.

The Programmable Search JSON API was the remaining route, and ADR-0019 was written partly to unblock it.
Checked against Google's own documentation on 2026-09-24, it no longer is one:

- The API is closed to new customers: "The Custom Search JSON API is closed to new customers."
- Existing customers lose it on 2027-01-01: they "have until January 1, 2027 to transition to an alternative solution."
- Since January 2026 a new Programmable Search Engine can search at most 50 domains. "Search the entire web" is not offered to new engines, and existing whole-web engines lose it on the same date.

Google's suggested replacement is Vertex AI Search, an enterprise Google Cloud product that also doesn't search the whole web.

## Decision

### 1. No Google source is built

A new JobPilot user cannot obtain a key for the API, and one who already holds a key could use a connector for about three months.
That doesn't justify a connector, its tests, or a line in the Settings source list.

Scraping stays out for the reasons above, and Vertex AI Search is out because it needs a Google Cloud project and billing account, and still doesn't search the whole web.

### 2. The need is still real, and Brave Search is where it will be revisited

The need is postings that live on a company's own hiring page and nowhere else.
The Hacker News source serves part of it.

Brave Search API is the closest legitimate replacement: an official, keyed web-search API with its own index.
The shape that fits the need is a query restricted with `site:` to the applicant-tracking hosts that serve company hiring pages — Greenhouse, Lever, Ashby and similar — so every result is a posting rather than an article about one.

It is deferred, not decided, because what it returns has not been measured:

- A search result is a title, a URL and a snippet, not a posting. Each hit would need its page read, the way a custom job URL is read today (AC-007 in `04_job_discovery.md`), which puts the model and possibly the browser on the discovery path for the first time.
- How many results a `site:` query returns per ATS host, and how fresh they are, decides whether it is a source at all or a search box.
- Its quotas and free tier decide whether a Scout run can use it without the user paying.

A decision to build it should start from those measurements, recorded the way the existing sources' were.

## Consequences

- The key store from ADR-0019 has no Google consumer. Its first keyed source is expected to be Adzuna, an established job-search API, when the user has a key.
- `google-search` is removed from the Settings suggestions. A stored credential under that name is not deleted, and nothing reads it.
- The five public sources remain the whole of discovery until a keyed source ships.

## Alternatives considered and rejected

**Build it anyway for existing key holders.** Three months of use, for a connector that would then fail on every run and need removing.

**A 50-domain engine limited to ATS hosts.** It would fit the need, and it is closed to new customers like the rest of the API.

**A third-party Google results API** that scrapes Google on JobPilot's behalf. It is the scraping this product refuses, moved to someone else's servers.
