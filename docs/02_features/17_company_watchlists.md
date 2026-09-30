# Feature: Company Watchlists

## Status

Implemented.
D1 to D6 were decided on 2026-09-30, and ADR-0024 records storage and how the feature is built, written with the implementation, as ADR-0022 was for About you.
It builds on ADR-0023, which reads where a job can be done from; this feature supplies jobs for that reading from a new kind of source.
"What building it changed", at the end, records where measuring the providers changed what this spec said.

## Purpose

Let the candidate name companies they want to work for, and find those companies' openings on the companies' own hiring pages, where every posting a company has is listed and nothing sits between the posting and its application form.

The job boards JobPilot reads carry a fraction of what companies post, and what they carry is second-hand: a board copies a posting, sometimes without its locations, and links out to the original.
ADR-0020 records the need this answers — postings that live on a company's own hiring page and nowhere else — and why Google is not the way to reach them.
Most companies host that page on an applicant-tracking system with a public, documented API for exactly this: building a careers page from the company's postings.

## What exists today

Measured on 2026-09-30.

The three providers below publish every company's postings as JSON, with no key:

| Provider | Endpoint | What says where a job is |
| --- | --- | --- |
| Greenhouse | `boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true` | `location.name`, free text: "Remote, Canada; Remote, United Kingdom; Remote, United States" |
| Lever | `api.lever.co/v0/postings/{site}?mode=json`, and `api.eu.lever.co` for EU accounts | `workplaceType` (`on-site`, `remote`, `hybrid`, `unspecified`), `categories.allLocations`, and `country`, an ISO 3166-1 alpha-2 code |
| Ashby | `api.ashbyhq.com/posting-api/job-board/{name}?includeCompensation=true` | `workplaceType` (`OnSite`, `Remote`, `Hybrid`), `location`, `secondaryLocations`, and a structured `address` with its country |

What their documentation says about access:

- Greenhouse: "Job Board data is publicly available, so authentication is not required for any GET endpoints."
- Lever: "All job postings in the `published` state are publicly viewable", and only its POST endpoint needs a key.
- Ashby documents the endpoint for public job boards and states no authentication or limits.

Two are structured beyond anything a board gives: Lever names each job's country as a code, and Ashby says whether a job is remote and lists every location it is open in.
GitLab's Greenhouse board held 202 jobs, many listed for several countries at once.

The installation measured already points at such pages.
Of its 269 jobs, 16 link to a company's Greenhouse, Lever or Ashby page, naming 16 distinct companies, and the three jobs its user added by URL are all Ashby postings, ClickUp's among them.

Workable and SmartRecruiters also host hiring pages.
Probed the same day, neither answered in a way that could be built on — Workable's public endpoint returned no JSON, and SmartRecruiters returned no postings for the company tried — so they are left out until measured properly.

## Principles

- **The candidate chooses the companies.**
  JobPilot never adds a company to the list, never guesses one's hiring page from its name, and suggests only companies a measurement put on the starter list.
- **A watched company is a source, not a search.**
  Its jobs pass through the candidate's saved searches like any other source's, so watching GitLab does not import 202 jobs, and a Scout needs no change to use it.
- **Reading a hiring page contacts no one.**
  The request is the same public read a careers page makes, and nothing is sent to the company.
  AGENTS.md principle 5 is untouched: this finds jobs, it does not apply to them.
- **The company's own words win on location.**
  Each provider's own list of a posting's locations is read with ADR-0023's rules, like every other source's, and its workplace type becomes the job's remote type.
  Lever's `country` and Ashby's `address` are not used for where: measured, they name the posting's office or hiring entity (ADR-0024 §6).

## Behaviour

### Watching a company

The candidate adds a company by pasting the address of its hiring page or of any one of its postings:

- `boards.greenhouse.io/{token}`, `job-boards.greenhouse.io/{token}`, and their `.eu` forms
- `jobs.lever.co/{site}` and `jobs.eu.lever.co/{site}`
- `jobs.ashbyhq.com/{name}`

Core reads the provider and the board's name from the address, deterministically, and checks it once by fetching the board.
The company's name is taken from the board where the provider gives one, and otherwise from the address, and the candidate can rename it.
An address from any other host is refused with the list of what is supported, rather than fetched.

A job whose URL, or whose description's application link, is on a supported board offers "Watch this company" on its page.
That is how the 16 companies above would be added: from jobs the candidate has already seen.

### Checking a watched company

Discovery asks the watched companies as one source, "Company pages", alongside the job boards.
Each board is fetched once, at most four at a time, and each answer is cached for an hour, so running every saved search asks each company once.
A board that has gone — a company that moved provider or closed its board — is reported by name, as a failing source is today (`04_job_discovery.md` AC-008), and stays on the list until the candidate removes it.

A job from a company's page keeps that page's URL as its canonical URL.
The provider's workplace type becomes `RemoteType`, and its locations become `Locations`, both in the source's own words, for ADR-0023 to read.

### Where it is managed

- Desktop: Settings → Job searches gains "Companies you watch": each company, its provider, when it was last checked and whether that worked, with Add and Remove, and below it the starter list's suggestions, each with what was measured and when.
- CLI: `jobpilot-cli company watch <url>`, `company list` and `company unwatch <id>`.
- CLI: `company suggestions` lists the starter list with what was measured and each suggestion's address, which is what `company watch` takes.
- Removing a company leaves its jobs where they are, as removing a saved search does; deleting data follows ADR-0021 (D4).

## Decisions

D1 to D6 were decided on 2026-09-30.

### D1. A company is added by address, or from a job

The candidate pastes the address of a hiring page or posting, or presses "Watch this company" on a job whose link is on a supported board.
Adding by name was rejected: there is no directory to look a name up in, so it would mean trying `{name}` against each provider and taking whichever answers, which finds the wrong "Close" or "Zoe" as readily as the right one.

### D2. JobPilot suggests companies from a measured starter list

The product ships a list of companies to suggest, and a company earns its place by measurement, not by reputation.
See "The starter list" below.
A list of companies "known to hire globally", kept by hand, was rejected: it is a claim about each employer that goes stale in the repository with nothing to show when it stopped being true.

### D3. A watched company's jobs go through the saved searches

They are a source like any other, so a Scout, a launch refresh and Discover all see them with no change, and a candidate who watches GitLab sees GitLab's engineering jobs, not its sales jobs.
Storing every posting of a watched company was rejected: it fills the local database with jobs no search asked for, which `SearchCriteria.IsEmpty` exists to prevent.

### D4. A `watched_companies` table, recorded in an ADR

It holds the provider, the board's name, the display name, the address it was added from, when it was added, when it was last checked and how that went, and how many postings the board held then.
It is the candidate's own data and gains a record kind in ADR-0021's deletion rules: deleted with "delete all data", not with a profile.
The ADR is written with the implementation, as ADR-0022 was.
A setting holding a list of addresses was rejected: it has no room for when each was checked, and it is not something ADR-0021 knows how to delete.

### D5. No limit on how many companies

Boards are fetched at most four at a time and each answer is cached for an hour, so a hundred companies is a hundred small requests an hour at most, each to a server built to answer them.
A cap was rejected as a rule the candidate would hit for no reason they could see.

### D6. Greenhouse, Lever and Ashby first

They are the three measured.
Workable and SmartRecruiters follow once their public endpoints have been measured and recorded here.

## The starter list

The starter list is suggestions: nothing on it is watched until the candidate picks it.

It is generated, never edited by hand.
A command in the repository reads a seed file of boards to consider — provider and board name, nothing else — fetches each board, and reads each open posting with ADR-0023's rules.
A company is kept when at least one of its open postings can be done from anywhere, or from countries on more than one continent.
Every other seed entry is dropped from the output, and the seed file says nothing about any company, so adding one to it claims nothing.

Each entry records what earned it its place and when:

- the provider, the board's name and the company's name as its board gives it
- the date it was measured
- how many postings were open, and how many of them could be done from anywhere or from more than one continent

The desktop shows that record as it is — "12 of 40 open roles could be done from anywhere or from more than one continent, measured 2026-09-30" — and never restates it as a property of the company.
The list is regenerated before each release, so no entry the product ships is older than the release that ships it, and an entry whose company no longer qualifies leaves the list at the next regeneration.
A company the candidate already watches is not suggested.

## Acceptance criteria

### AC-001
Given the address of a Greenhouse, Lever or Ashby hiring page or posting, when the candidate watches it, then the company is added once its board has answered, named from the board, and an address from any other host is refused with what is supported.

### AC-002
Given a job whose URL or application link is on a supported board, when its page is shown, then it offers "Watch this company", unless the company is already watched.

### AC-003
Given watched companies, when a saved search runs, then their postings are among the results under the same criteria as every other source's, each board fetched at most once an hour.

### AC-004
Given a job from a company's page, then its canonical URL is that page's, and where it can be done from is read from the provider's structured fields when it has them and from its location text otherwise (ADR-0023).

### AC-005
Given a watched company whose board no longer answers, when discovery runs, then the company is named as failing, the other sources' results are kept, and the company stays on the list until removed.

### AC-006
Given the list of watched companies, the candidate can see when each was last checked and whether that worked, rename one, and remove one, from the desktop and from the CLI.

### AC-007
Given the starter list, when the candidate opens their watched companies, then each suggestion not already watched is shown with how many of its open roles could be done from anywhere or from more than one continent and the date that was measured, and choosing one watches it.

### AC-008
Given the seed file, when the starter list is regenerated, then it holds only the companies whose boards answered with at least one posting that ADR-0023's reading places anywhere or on more than one continent, each with its measurement and date.

## Out of scope

- Applying through a provider's API.
  Lever and Greenhouse both take applications by POST with the company's key; JobPilot never submits (ADR-0014), and has no such key.
- Workable, SmartRecruiters and other providers, until measured (D6).
- Refreshing the starter list from inside the product.
  It is regenerated in the repository, and a candidate's installation never measures boards it has not been asked to watch.
- Merging a job found on a board with the same job found on the company's page.
  They have different URLs, so today they are two jobs; recognising them as one is a question for AC-005 of `04_job_discovery.md`, not for this feature.

## What building it changed

Built on 2026-09-30.
The decisions hold; measuring the three providers changed four things this spec said, each recorded in ADR-0024:

- **Structured locations are not where a job can be done from (§6).**
  Ashby's `address` and Lever's `country` name the office or hiring entity: Railway's "Global" roles carry a San Francisco address, and Toptal's "Europe and Canada" roles GB.
  Each provider's list of locations is read instead.
- **Lever and Ashby name the company on their hosted pages, not in their APIs (§4).**
  The name is read from the page once, when a company is watched.
  Lever's API answers a site that does not exist with an empty list, and its page with 404, so the page also decides whether the board is there.
- **A starter entry needs its board to name the company.**
  PostHog's and Cursor's Ashby pages give no name, so the generator drops them rather than suggest a company it cannot name.
- **The starter list is ordered by how many roles qualified**, so the suggestions read first have the most to offer; measured on 2026-09-30, 38 of 87 seed boards qualified, from Canonical's 274 of 306 roles to one role at several.

Measuring also found two mistakes in ADR-0023's reading, fixed with it, so `SignalsVersion` is 2 and every stored job is read again:

- "Georgia" beside the United States, or a city there, is the US state: "Remote, US, Georgia" had read as Georgia the country, which put Celonis and Elastic on the list.
- A three-letter name is a city only when it is the city's own, like Ulm: GeoNames lists airport codes among cities' names, and Stripe's "Tor", for Toronto, had read as Slovyansk.
  "NYC" and "SF" are kept by hand.
- "AMER" is the Americas.
