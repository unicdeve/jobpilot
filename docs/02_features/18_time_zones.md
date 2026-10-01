# Feature: Time-zone requirements

## Status

Implemented.
D1 to D5 were decided on 2026-10-01, each as recommended, and ADR-0025 records how they are built.
ADR-0023 left a job's time-zone requirement unread (its "Consequences") and asked for it to be measured as its own decision; this is that measurement.

## Purpose

Say, on each job, the hours or time zones it asks the candidate to work in, and let a saved search leave out the jobs a candidate cannot work for.

A role open "anywhere" is often open anywhere within a working day.
"Anywhere in the World" with "Located in CET timezone (+/- 3 hours), we are unable to consider applications from candidates in other time zones" is open to Lagos and closed to Manila, and nothing in its location says so.

## What exists today

Measured on 2026-10-01, over 1,657 postings: the five newest Himalayas search pages for "engineer", Jobicy, Remotive, We Work Remotely's engineering feeds, and the Greenhouse, Lever and Ashby pages of GitLab, Canonical, Remote, Fivetran, Toptal, Supabase, Oyster, Zapier, Railway, ClickHouse, Linear, ElevenLabs and n8n.

### Himalayas' field says nothing its location does not

Himalayas gives every job `timezoneRestrictions`, a list of UTC offsets.
It is derived from `locationRestrictions`, measured on all 99 jobs read: the United States is always `[-10 … -5, 14]`, Germany `[1]`, India `[5.5]`, and a worldwide role every offset there is.
ADR-0023 already reads the location, so this field is not used.

### Requirements are sentences

112 postings (7%) say something about the hours or zones they need, beyond "we work across time zones".
14 of the 231 labelled anywhere, worldwide or global (6%) do: the label says the least exactly where this matters most.

They are written five ways:

| Kind | Postings | As written |
| --- | --- | --- |
| A zone region | 64 | "These roles are home based in the EMEA time zone", "This is a remote role in Americas time zones", "required location in European time zones" |
| Working hours in a zone | 20 | "Mandatory availability from 9:00 AM – 12:00 PM PST", "This position requires working hours in CEST", "100% remote in LATAM working EST hours" |
| An offset range | 19 | "Located in CET timezone (+/- 3 hours)", "based within UTC−6 to UTC+3", "+3 / -7 UTC", "between UTC-4 and UTC+4", "within one hour of CET" |
| A preference | 12 | "EST time zone is preferred", "we prefer candidates based in GMT+0 to GMT+3" |
| Overlap with a zone's day | 7 | "Able to overlap at least 4 hours a day with US Eastern Time", "at least 7 hours of overlap with the 9 am-5 pm EST workday", "Overlap: 6hs with East Coast time zone" |

Many more sentences only mention time zones and ask nothing: "collaborate asynchronously across time zones" (most of GitLab's), "we have remote roles open in every time zone" (Canonical's), "flexible working hours" (a benefit).
They are the bulk of the matches, and reading them as requirements would be the failure this feature has to avoid.

Counts are from patterns written to find each kind and checked by hand against the sentences; they are a floor, not an exact census.

### For a candidate in Nigeria

Nigeria is UTC+1, which is CET in winter.
Of the hard requirements above, the European ranges ("CET ± 3", "UTC−6 to UTC+3", "GMT+0 to GMT+3") include it, and so do EMEA zone regions.
The US ones — overlap with Eastern time, Pacific working hours — do not, unless the candidate works shifted hours, which many remote workers in West Africa do for US employers.

## Principles

- **A requirement is read, never inferred.**
  A job that names no zone or hours has no requirement; its location is not turned into one, which is what Himalayas does.
- **Asking nothing is not a requirement.**
  A sentence about working across zones, a benefit of flexible hours, or a team's spread is not read, and when unsure the job is left without a requirement rather than given a wrong one.
- **The sentence is shown.**
  As with relocation and visa sponsorship (ADR-0023 §5), the job shows the sentence its requirement was read from.
- **Where and when are separate facts.**
  "Anywhere" stays anywhere; a time zone is a second line, not a narrower location.

## Decisions

D1 to D5 were decided on 2026-10-01; each took the recommended option, marked below.

### D1. What a requirement becomes

The five kinds have to become something a candidate's time zone can be compared with.

- **A. One shape: a window of UTC offsets the candidate must be in.** (decided)
  A range is its window; "CET ± 3" is UTC−2 to +4.
  A zone region is a fixed window: EMEA UTC−1 to +4, Americas UTC−10 to −3.
  Overlap of *n* hours with a zone's 9-to-5 is that zone's offset ± (8 − *n*): "4 hours with US Eastern" is UTC−9 to −1.
  Fixed working hours are kept as hours and shown, since they ask for a schedule rather than a place, and compared as overlap with the hours they name.
- **B. Keep each kind as its own type**, compared by its own rule.
  More faithful to the sentence, and five comparisons where A has one.
- **C. Only zone regions and ranges**, leaving overlap and fixed hours unread.
  Simplest, and misses the US-hours requirements that matter most to a candidate in West Africa.

### D2. Where the candidate's time zone comes from

- **A. The profile gains a time zone**, chosen from a list, prefilled from the profile's country when the country has one zone, and from the computer's otherwise. (decided)
- **B. Derived from the profile's country** alone.
  Wrong for the United States, Canada, Brazil, Russia and Australia, among others.
- **C. A saved-search criterion**, set per search.

### D3. Whether a requirement filters a search

- **A. Shown on every job, and a saved search can leave out the ones the candidate cannot work for, allowing them to shift their day by up to a number of hours they choose.** (decided)
  "I can shift my hours by 5" keeps a US Eastern overlap requirement for a candidate in Lagos.
  Off until the candidate turns it on, as relocation is.
- **B. Shown only, never filtered.**
- **C. Filtered strictly by the candidate's own zone**, with no shift.
  Drops every US-hours role for a candidate in West Africa, including the ones they would take.

### D4. Preferences

- **A. Shown, never filtered.** (decided)
  "EST preferred" does not exclude anyone, and treating it as a requirement would hide jobs the candidate can apply for.
- **B. Treated as requirements.**

### D5. A label of anywhere with a zone in the description

Canonical labels roles "Home based - Worldwide" and says "These roles are home based in the EMEA time zone".

- **A. The job stays anywhere, with its requirement beside it.** (decided)
  Where it can be done from and when it has to be done are two facts, and D3 filters on the second.
- **B. The requirement narrows the job's location**, so "anywhere" with "EMEA time zones" becomes listed for EMEA.
  It merges two facts into one and loses which the posting said.

## Acceptance criteria

### AC-001
Given a posting that states a range, a zone and a margin, an overlap with a zone, hours of the clock in a zone, a zone's working hours, or a region's time zones, when it is read, then it requires the window ADR-0025 §1 gives, with the sentence it was read from.

### AC-002
Given a posting whose sentences only mention time zones — working across them, a team's spread, flexible hours, a shift rota — when it is read, then it has no working hours.

### AC-003
Given a posting that only prefers hours, then they are shown as preferred and no search leaves it out.

### AC-004
Given a profile with no time zone, when it is edited, then the zone is prefilled from its country when the country has one zone and from the computer otherwise, and a zone JobPilot does not know is refused.

### AC-005
Given a saved search that fits hours, with a shift of *n* hours, when it runs, then a job whose required window is more than *n* hours from the profile's zone is left out, and every other job is kept, including all of them for a profile with no zone.

### AC-006
Given a job with working hours, when its page is shown, then the window, whether it is required, the sentence, and how far it is from the profile's zone are shown, and where it can be done from is unchanged.

### AC-007
Given stored jobs read before this reading existed, when the daemon starts, then each is read again.

## Out of scope

- Shift work and on-call rotas ("16:00–02:00 UTC", "Friday → Monday") as schedules; they are shown as the posting wrote them.
- Daylight saving as a requirement: a window is compared at standard offsets, so "CET" and "CEST" read the same.

## What building it changed

Built on 2026-10-01.
The decisions hold; reading the measured sentences refined three things, each in ADR-0025 §1:

- **Noise is checked after numbers.** "Located in CET timezone (+/- 3 hours), we are unable to consider applications from candidates in other time zones" had been dropped as a sentence about other time zones.
- **Named zones win over regions in one sentence.** "Anywhere in the United States, however the ability to operate in ET, CT, MT, or PT timezones is required" asks for the four zones, not every US offset.
- **"GMT+7" is a zone.** "At least four hours of weekday overlap with Bangkok (GMT+7)" had been read against GMT.

Over the 1,657 postings, 105 require hours and 15 prefer them; of the 13 labelled anywhere that require hours, each was checked by hand.
For a candidate in Lagos, 58 of the 105 are outside their zone, and 37 still are with a five-hour shift.
