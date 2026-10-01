# ADR-0025: The working hours a job asks for

## Status

Proposed, and implemented with it.
It records `docs/02_features/18_time_zones.md` D1 to D5 (decided on 2026-10-01), and how they are built.
It reads one more signal beside ADR-0023's, by the same rules: from the job's own text, deterministically, re-read when the reading changes.

## Context

ADR-0023 left a job's time-zone requirement unread, and the spec measured it: 105 of 1,657 postings require hours or zones and 15 prefer them, and the requirement sits as often on a job labelled anywhere as on any other.
Himalayas' `timezoneRestrictions` is derived from its location field and adds nothing.
So the requirement is read from sentences, as relocation and visa sponsorship are, and compared with a time zone the candidate gives.

## Decision

### 1. A requirement is one window of UTC offsets

```go
type WorkingHours struct {
    Status   HoursStatus // "required", "preferred", or "" for none
    From, To float64     // UTC offsets in hours, without daylight saving
    Evidence string      // the sentence it was read from
}
```

`domain.readHours` reads each sentence of the description into a window (spec D1):

| As written | Window |
| --- | --- |
| Two offsets: "UTC−6 to UTC+3", "+3 / -7 UTC", "between UTC-4 and UTC+4", "GMT to GMT+3" | the two |
| A zone and a margin: "CET (+/- 3 hours)", "within one hour of CET" | the zone ± the margin |
| Overlap: "at least 4 hours a day with US Eastern Time" | the zone ± (8 − hours): a candidate's 9-to-5 overlaps that much |
| Clock hours: "9:00 AM – 12:00 PM PST" | where a candidate's 9-to-5 covers those hours |
| A zone's working hours: "working hours in CEST", "operate in ET, CT, MT, or PT timezones" | the zones named |
| A region as zones: "home based in EMEA time zones" | a fixed window per region: EMEA UTC−1 to +4, Americas UTC−10 to −3, APAC UTC+5 to +13 |

Zone abbreviations are read in capitals only — "ET" is French for "and" in Canonical's postings — at standard offsets, so "CET" and "CEST" are one zone.
A sentence about working across zones, a team's spread or flexible hours asks nothing, and is only checked for after the readings that need numbers: "CET (+/- 3 hours), we are unable to consider candidates in other time zones" is a requirement.
Shift rotas are not read (spec, out of scope).
A window 20 hours or wider asks nothing.
A sentence with a word of preference ("preferred", "ideally", "we encourage") is a preference.
A posting's required windows are joined, since it accepts any of them, and a requirement wins over a preference (spec D4).

The reading is tested against the measured sentences, both those that ask and those that do not.

### 2. The candidate's time zone is on the profile

`candidates.time_zone` holds an IANA zone, such as `Africa/Lagos` (spec D2).
Its offset comes from GeoNames' `timeZones.txt`, which `gengeo` now compiles into `geo` beside the countries and cities: 418 zones, each with its country and standard offset.
No tz database is read, so the daemon is the same on every platform.

The desktop offers the profile country's zones first and prefills a profile with none: the country's zone when it has one, as 216 countries do, and the computer's otherwise.
The daemon refuses a zone GeoNames does not list.

### 3. A saved search can fit the hours

`SearchCriteria` gains `FitHours` and `HoursShift`, 0 to 12, stored on `saved_searches` (spec D3).
With `FitHours`, `MatchesFor` leaves out a job whose required window is more than `HoursShift` hours from the candidate's zone, measured around the clock.
A preference, a job that says nothing, and a candidate with no zone leave nothing out.
`FitHours` alone does not make criteria non-empty, since it keeps nearly every posting.

`MatchesFor` takes a `domain.Seeker` — the candidate's authorized countries and time zone — in place of the list of countries, so a Scout and discovery pass the same facts.

### 4. Stored and re-read like the other signals

`jobs` gains `hours_status`, `hours_from`, `hours_to` and `hours_evidence` (migration `00021`).
`SignalsVersion` becomes 3, so every stored job is read again on the next start.
Where a job can be done from is unchanged by its hours: an anywhere job with "EMEA time zones" stays anywhere, with its hours beside it (spec D5).

## Consequences

- One migration, three tables: the job's hours, the profile's zone, two saved-search criteria.
- `GET /v1/countries` also returns every time zone, so no client holds the table.
- The stored jobs list does not filter by hours; a saved search and a Scout do. It can follow if a list filter is asked for.
- The window is approximate where postings are: "overlap with working hours in the Eastern time zone" names no hours and is read as that zone alone, which a candidate's shift then widens.

## Alternatives considered

**Each kind kept as its own type** (spec D1 B).
More faithful, and five comparisons where one window serves; every kind measured converts without loss a candidate would notice.

**Himalayas' offsets.**
Derived from its location, which ADR-0023 already reads.

**Go's tz database for offsets.**
It differs by platform, and on Windows needs the database embedded; GeoNames' table is already the source of every other place JobPilot knows.
