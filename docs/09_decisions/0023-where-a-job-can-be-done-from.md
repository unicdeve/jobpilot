# ADR-0023: Where a job can be done from, and whether it helps you move

## Status

Accepted, and implemented.
"What building it changed" at the end records where the implementation refined a decision below.
It covers four changes to discovery, specified as AC-010 to AC-013 in `docs/02_features/04_job_discovery.md`:

1. A job says where it can be done from, and a saved search says where the candidate is.
2. Himalayas is read through its search endpoint rather than its newest pages.
3. We Work Remotely and Jobicy become sources.
4. A job says whether it offers relocation and visa sponsorship.

Company watchlists, which read employers' own hiring pages, are a separate feature: `docs/02_features/17_company_watchlists.md`.

## Context

A candidate looking for a remote role they can take from their own country, or for a role abroad that helps them move, cannot ask JobPilot for either.
Two things stand in the way, and both were measured on 2026-09-30 against the live sources and a real installation's database of 269 jobs.

### Discovery reads a thin slice of each board

No source filters server-side, so each search reads what the source hands back and filters it locally:

| Source | Jobs the source holds | Jobs one search reads |
| --- | --- | --- |
| Himalayas | 92,862 | 80, its four newest pages |
| RemoteOK | one feed | 99 |
| Remotive | one feed | 16 |
| Arbeitnow | paged | 326, its first page |

Himalayas has a search endpoint that the connector does not use.
`/jobs/api/search?q=backend engineer&country=NG` returns 219 roles open to someone in Nigeria, counting both worldwide roles and roles that name the country; `q=software engineer&worldwide=true` returns 422.
The installation measured holds 30 Himalayas jobs in total, 13 of them restricted to the United States.

### "Remote" is one word, and location is a substring

`Job.RemoteType` is `remote`, `hybrid` or `onsite`, and `Job.Locations` holds whatever text the source gave.
`SearchCriteria.Location` matches that text by substring.
So neither setting a location nor leaving it blank finds the jobs a candidate can take:

- **Blank**, every remote job matches, including the ones stored as "Remote (US)", "US (Fully Remote)" and "USA".
- **"Nigeria"**, every job that says "Worldwide", "Remote (Global)", "REMOTE (anywhere, EST hours)", "Remote(Everywhere)" or "Anywhere in the World" is hidden, because it does not contain the word.

`Locations` also means two different things.
On a remote board it is where the candidate may be: Remotive's `candidate_required_location`, Himalayas' `locationRestrictions`.
On Arbeitnow and for most Hacker News postings it is where the office is, and it is usually a city with no country: 147 of the 269 jobs are onsite Arbeitnow roles, in "London", "Berlin", "München", "Zürich".

### A board's label is not the last word

We Work Remotely labels every posting with a region, and on 2026-09-30 its engineering feeds held 118 postings labelled "Anywhere in the World".
Eight of them say otherwise in the description: "Candidates must reside in or be willing to relocate to the San Francisco Bay Area", "Applicants must be able to work legally within the United States", "US based applicants only".
Himalayas' empty `locationRestrictions` means worldwide, which its `worldwide=true` search confirms, and Jobicy's `jobGeo` is one of its own slugs: "Anywhere", "EMEA", "USA".

### Relocation and visa sponsorship are only ever sentences, and often refusals

No source has a field for either.
Across 852 postings — the stored jobs plus the two proposed sources' feeds — "relocat" appears 41 times and "visa" or "sponsor" 138 times.
Read by hand, relocation sentences split into three roughly equal groups:

- **Offers**: "we offer you an attractive relocation package", "Relocation support (up to €10k)", "Full relocation to SF covered".
- **Refusals**: "We are currently unable to offer fully remote work, visa sponsorship, or relocation support", "Relocation assistance will not be provided for this role".
- **Requirements that cost the candidate**: "Based in or willing to relocate to San Francisco", with "may be available" and "position dependent" between them.

Visa mentions are mostly refusals: "we are unable to sponsor visas", "No visa sponsorship", "NO VISA (EU work authorisation required)".
Several matches are not about visas at all: "mentor, sponsor and develop senior engineers", "company-sponsored dinners", "employer-sponsored 401(k)".
A badge for the keyword would call most of these jobs the opposite of what they say.

### A country is not enough to place an office

Resolving "Berlin" or "Zürich" needs a city table.
Measured against the 169 onsite and hybrid location strings stored, a GeoNames table of cities over 100,000 people places 138 of them in a country; nearly all the rest are country names ("United Kingdom"), which a country table places.
Lowering the threshold to 15,000 people adds four resolutions and triples the table.
Raising it to 500,000 loses Zürich.

## Decision

### 1. A job says where it can be done from

Core derives, for every job, where the candidate may be to take it:

| Scope | Meaning | Example source text |
| --- | --- | --- |
| `anywhere` | no restriction on where the candidate is | "Worldwide", "Anywhere in the World", "Remote (Global)" |
| `listed` | these countries, or countries in these regions | "Remote (US)", "EMEA", "USA, Canada, Argentina, Mexico, Peru", or, for an onsite job, "München" |
| `unknown` | the source did not say, or said something core does not read | no location; "Full-time*"; a board label a description contradicts |

What `Locations` means follows `RemoteType`, and connectors keep writing the source's own words into it:

- For a **remote** job, `Locations` is where the candidate may be.
  A connector whose source encodes "worldwide" as an absence writes it out: Himalayas' empty `locationRestrictions` becomes "Worldwide".
- For an **onsite or hybrid** job, `Locations` is the office, and the job is `listed` for the office's country.
  Being there is how it can be done.

Reading the text is deterministic and in core, one function beside `SearchCriteria.Matches`.
It is not delegated to the model (AGENTS.md principle 7), and the discovery path stays without one: several hundred jobs per search, from untrusted text.
The vocabulary is built from measured strings, the way the Hacker News convention was: the words for anywhere, the named regions, country names and their common forms ("USA", "U.S.", "UK", "Türkiye"), and cities.
A phrase core does not recognise leaves the job `unknown` rather than guessed at.

**A title or a description can make a job less certain, never more.**
When a job reads as `anywhere` or `listed` and a sentence in its description restricts where the candidate must be — "must be located in", "US based applicants only", "authorized to work in the United States", "remote within the U.S." — the job becomes `unknown`, and that sentence is kept as its location note.
So does a title that names a place — "Staff Backend Engineer (Europe)" — on a job labelled anywhere.
For a `listed` job, a title or sentence is consistent when any place it names is among the job's own: "must be within EMEA" agrees with Himalayas' list of 149 EMEA countries, though its EMEA and JobPilot's differ at their edges.
Only one naming none of them says the label is wrong.
It is not narrowed to the places the sentence names.
A sentence read wrongly would then hide a job the candidate could have taken, and the discovery spec already holds that a wrong location silently hides a job while a missing one leaves it visible.
The note is shown, so the candidate sees why.

### 2. Countries, regions and cities are core's data

`internal/core/domain/geo` holds three tables, compiled in:

- **Countries**: ISO 3166-1 alpha-2 codes, English short names, and the common alternative forms measured in postings.
- **Regions**: each region a set of countries — Europe, the EU, EMEA, Africa, the Middle East, APAC, Asia, LATAM, the Americas, North America, Oceania, DACH, the Nordics — built from the UN M49 regions where one exists and written out where a region is a business convention (EMEA, APAC, LATAM, DACH).
- **Cities**: GeoNames `cities15000`, filtered to cities of at least 100,000 people, keyed by name and Latin-script alternative names, each giving every country with a city of that name, most populous first, and the English name of that city.
  An ambiguous name resolves to its most populous city — "London" is the United Kingdom, "Paris" is France — unless the rest of the location says otherwise: "Cambridge, MA" is in the United States and "London, ON" in Canada, while "Hamburg, DE" stays in Germany, since a state's code only chooses among the cities a name already has.
  It is 65,193 names, 412 KB compressed, embedded with `go:embed`.

The tables are generated by `go generate` in the package, never fetched at runtime, so placing a job needs no network and sends nothing anywhere.
GeoNames publishes its dumps daily without versions, so a generation records the SHA-256 of each input and the date it was made.
GeoNames is licensed CC BY 4.0, and is credited in the README's "Third-party data" and in the package itself.

ADR-0015 stores a work authorization's country "as the user writes it", with "no country database here".
That stays as stored, and now there is one to read it with: section 5 resolves an authorization's country through the same table.

### 3. What is derived is stored, and re-derived when the reading changes

A job stores:

- `workable_scope`: `anywhere`, `listed` or `unknown`
- `workable_note`: the sentence that made it `unknown`, when one did
- `workable_countries`, the ISO codes of a `listed` job, regions expanded, written with a comma on each side (`,DE,FR,`)
- `relocation` and `visa_sponsorship`, each `offered`, `mentioned`, `declined` or empty, and the sentence each came from (section 5)
- `signals_version`, the version of the reading that produced them

Expanded countries let the stored jobs list filter in SQL, as it filters every other criterion, with `LIKE '%,DE,%'`, and let `Matches` compare country codes without reading text.
A column rather than a table of rows keeps the deletion rules of ADR-0021 as they are: the list scans its other filters already, and a job carries its countries with it.
The derivation reads only fields a job already stores, so when the vocabulary, the tables or the phrases change, the daemon re-derives every job below the current version on starting, in batches, and a fix reaches old jobs as well as new ones.

### 4. A saved search says where the candidate is, and whether they would move

`SearchCriteria` gains two fields:

- `work_from`, an ISO 3166-1 alpha-2 country code: "I can work from"
- `include_relocation`, a boolean: "Also include jobs elsewhere that offer relocation or visa sponsorship"

With `work_from` unset, a search behaves exactly as it does today, so an existing saved search changes only when the candidate edits it.

With `work_from` set, `Matches` keeps a job when:

- its scope is `anywhere`
- its scope is `listed` and `work_from` is among its countries
- its scope is `unknown` — missing data does not exclude a job, which is the rule `withinSalary` already follows — and the job is shown as not saying where it can be done from
- `include_relocation` is set, `relocation` or `visa_sponsorship` is `offered` or `mentioned`, and it is not excluded by the rule in section 5

It is still the one definition of what a search means, so Discover, the stored jobs list, a Scout and a launch refresh agree.
`SearchCriteria.Location` stays what it is: text the job's location must contain, for a candidate who wants a particular city.

Search templates (`docs/02_features/14_search_templates.md`) do not set `work_from`, since a location belongs to the user.
A new saved search starts with the profile's `location_country` when that resolves to a country, whether it is written in the form or adopted from templates, and the candidate can change it.
The daemon serves the countries and the profile's own as a code (`GET /v1/countries`), so no client holds a copy of the tables.

### 5. Relocation and visa sponsorship are read sentence by sentence

Core reads a job's description a sentence at a time, and each sentence a clause at a time, split at "but", "however" and semicolons: "Visa sponsorship may be available; relocation is not provided" mentions one and refuses the other.
A sentence counts for relocation when it contains a relocation term, and for visas when it contains "visa" or "sponsorship" together with a work term ("visa", "work permit", "right to work", "authorization"), so "company-sponsored dinners" is not about visas.
Each counting sentence is classed:

- **declined**, when it carries a refusal of the help itself: "unable to offer … relocation support", "will not be provided", "not eligible for", "not offering", "no visa", "without sponsorship" — so "we are not a typical company, and we offer relocation support" refuses nothing
- **offered**, when it states an offer and no refusal: "relocation package", "relocation support", "visa sponsorship available", "Visa sponsorship: yes"
- **mentioned**, for anything else, including a conditional ("may be available", "position dependent"), a question, and a requirement at the candidate's cost ("willing to relocate")

A job takes the strongest class any of its sentences has, in the order declined, offered, mentioned, and keeps that sentence as its evidence.
Declined wins, because a posting that refuses in one sentence and advertises benefits "that may include relocation" in another has refused.
The class is a label for the candidate, always shown with its sentence, never a claim that a package exists.

When `include_relocation` brings in a job elsewhere, it is still excluded if its visa sponsorship is `declined` and the candidate has no work authorization for any of its countries (ADR-0015), resolved through the country table.
That is the one place a refusal hides a job, and it hides it only from a search that asked for jobs requiring a move.

### 6. Himalayas is searched, not browsed

When a search has a title or keywords, the Himalayas connector calls `/jobs/api/search` with them as `q`, and with `work_from` as `country` when it is set; that endpoint returns worldwide roles along with those naming the country.
It reads up to five pages of 20, the endpoint's page size.
A search without a title or keywords browses as it does today.

`FiltersServerSide` stays false.
Himalayas' `q` is its own relevance search, not `Matches`, so core still filters what comes back; the endpoint narrows what is read so that what core filters is the right slice of 92,862 jobs rather than the newest 80.
Results are cached per query for an hour: Himalayas updates daily, asks API users not to poll needlessly, and answers 429 when they do.

### 7. We Work Remotely and Jobicy are sources; Working Nomads is not yet

Both publish their feeds for this use, asking for attribution, and both label where a job can be done from.

- **We Work Remotely**: its category RSS feeds for programming, full-stack, back-end, front-end and DevOps roles, joined by URL.
  Its `country`, the employer's own list, goes into `Locations` when there is one, and its `region` otherwise.
  Measured on 2026-09-30, 21 of 63 postings labelled "Anywhere in the World" named countries, most of them the United States alone, and their descriptions agreed with the countries: "Location: Remote - US", "Authorization to work in the United States".
  Its terms: "Anyone can use the feed, all we ask is that you attribute the links back to We Work Remotely."
  Every job keeps its We Work Remotely URL as its canonical URL.
- **Jobicy**: `/api/v2/remote-jobs` with `count=200`, once for `geo=anywhere` and once for each of Jobicy's own locations that contains `work_from` — its country slug, if it has one, and its region, such as `emea` for Nigeria.
  The slugs are read from `?get=locations`, as Jobicy asks, and cached for a day.
  Jobicy's terms say automated checks "must not run more frequently than once per hour", so each URL is cached for at least an hour whatever asks for it: every saved search, a Scout, a launch refresh.
  A search therefore costs Jobicy one request an hour for each of those feeds — two for Nigeria, four for Germany — however many saved searches run, and a search without a country reads the newest jobs from everywhere, once.
  Every job keeps its Jobicy URL, which the terms require.

Working Nomads publishes a public feed with the same kind of labels ("Global", "Europe, LATAM, APAC, the U.S., Canada"), but no terms for it were found.
`docs/04_connectors/01_source_priorities.md` says not to assume a site permits automated access, so it is not built until it does.

Both new connectors report unreachable apart from failed, as every connector does (`docs/04_connectors/00_connector_sdk.md`).

## Consequences

- Discovery asks seven sources instead of five, and the desktop's "loading jobs from N sources" follows from the list.
- `jobs` gains the columns in section 3; `saved_searches` gains `work_from` and `include_relocation`.
  A migration adds them empty, and the first start derives every stored job.
- The binary carries about 420 KB of place names, and the repository a command that regenerates them from GeoNames.
- A Scout inherits both new criteria from its saved search, so its eligibility follows the same rules without a change of its own.
- The Jobs screen can show where a job can be done from and its relocation and visa labels, each with the sentence behind it, and the stored list gains the same filter as a search.
- A job's time-zone requirement ("CET (+/- 3 hours)", Himalayas' `timezoneRestrictions`) is not read.
  Many global roles ask for overlap with European or American hours, so it is the next thing to measure, as its own decision.
  It was measured and is now read: ADR-0025.
- Remotive's API now returns 16 jobs.
  It stays a source; whether it earns its place is a question for later, not for this decision.

## Alternatives considered

**Ask the model where a job can be done from.**
It would read the conversational cases better, and it would put a model call, and untrusted text, on the discovery path for every job of every search, to decide a rule that filters what the candidate sees.
Principle 7 keeps deterministic rules out of the model, and a phrase the vocabulary misses leaves a job visible, not hidden.

**A badge whenever a posting mentions relocation or visas.**
Measured, most visa mentions are refusals and a third of relocation mentions are, so the badge would be wrong more often than right.

**Trust each board's label.**
Eight of 118 "Anywhere in the World" labels on We Work Remotely are contradicted by their own description.

**Narrow a job to the places its description names.**
More precise when the sentence is read correctly, and it silently hides the job when it is not.

**Country names without cities.**
Most onsite locations are cities with no country, so a candidate who set `work_from` would still see every Berlin and London job, which is the problem this solves.

**A geocoding service.**
A network call per location, sending what the candidate searches for to a third party, for what a 289 KB table answers locally.

**All of GeoNames' cities over 15,000 people.**
Triple the size for four more of 169 locations.

**Read every page of Himalayas.**
Pulling a hundred thousand jobs into a local database on every search is a different product, as `himalayasMaxPages` already records; searching reads the right ones instead.

## What building it changed

Implemented on 2026-09-30.
The decisions above hold; building them against real postings refined five of them, and each is written into its section:

- **A title can contradict a label (§1).** Himalayas listed "Staff Backend Engineer - Core Product (Europe)" as worldwide.
- **Consistency is overlap, not containment (§1).** Requiring a sentence's places to fall inside the label's marked Hostaway's EMEA roles unclear, because Himalayas' EMEA is not JobPilot's.
- **Cities keep every country their name has (§2).** "On-Site (Cambridge, MA)" read as Cambridge in England, which would hide the job from a candidate in the United States. The table grew from 289 KB to 412 KB to hold it.
- **Countries are a column, not a table (§3).**
- **We Work Remotely's country list is used over its region (§7).**

Two things the ADR did not anticipate were added:

- Adopting a template gives each search the profile's country, since onboarding adopts templates rather than filling the form.
- Country names that hold a separator — "Bosnia and Herzegovina", ISO's "Taiwan, Province of China" — are read whole. Read in pieces, the second put a job in China.

Measured on a copy of a real installation: the 269 stored jobs were read again in under a second; a saved search for backend engineers from Nigeria then found 42 jobs, 32 of them new, among them roles Himalayas lists as open only in Nigeria.
