# Feature: Candidate Profile

## Purpose

Maintain structured candidate facts used by matching and application generation.

## Core data

A fact an application form asks for separately is stored separately (ADR-0015).
The shape below is what the recorded forms in `internal/adapters/secondary/browser/testdata/ats` actually ask for, and nothing more: a profile that models everything a CV might contain is a profile nobody finishes filling in.

- First name
- Last name
- Email
- Phone
- Location: city, state/region, country — asked for separately by real forms
- Availability: notice period in days, earliest start date
- Links, one per kind: LinkedIn, GitHub, portfolio, website, Twitter, other
- Work authorization, per country: whether the candidate may work there, and whether they need sponsorship
- Work preferences
- Skills, each with an optional number of years
- Work history, as dated entries: company, title, start, end, whether it is current, and what they did
- Education and certifications, as prose
- Application preferences (tone and length for generated content)

Education and certifications stay prose deliberately.
No recorded form asks about a degree, an institution or a certification, and nothing in JobPilot reads them, so structuring them would be schema for a requirement nobody has stated.
A recorded form that asks is the trigger to revisit.

Salary is deliberately absent.
A salary floor is a filter and belongs to the saved search, where `SearchCriteria` keeps it.
What a candidate tells an employer about their expectations is a different fact with a different audience, and JobPilot does not state it for them — a form's salary question goes to the user.

Self-identification data — gender, ethnicity, sexual orientation, transgender status, disability, veteran status — is not stored at all.
Every recorded form asks for it, it is optional on every one of them, and storing it to autofill it would be JobPilot deciding that answering is the default.

## Rules

Candidate profile data is factual source material. The LLM must not invent missing values.

First name, last name, email, phone, location, links, Summary, Skills and the
work history may be populated from an LLM-generated draft after resume import
(see `03_resume_management.md`), but only through the same explicit
review/edit step as manual entry — the LLM never writes to the profile
directly.

A proposed list only fills an empty one. Merging entry by entry would need a
notion of which proposed role is the same as one the user typed, and getting
that wrong silently rewrites their own history.

The draft may not estimate how long someone has used a skill. A duration the
resume does not state is left unstated, because a number nobody wrote down,
given to an employer, is a fact invented about a person.

Fields a resume wouldn't plausibly contain (work preferences, application
preferences) are never drafted this way, and neither is work authorization.
A resume does not reliably say which countries someone may work in, and a
guess there becomes a claim about their right to work, made to an employer,
under their name.

## Acceptance Criteria

### AC-001
User can create and edit a profile locally.

### AC-002
Profile data survives application restart.

### AC-003
Profile fields can be deleted or cleared.

### AC-004
Generated application content can reference profile facts but cannot silently modify the profile.

### AC-005
Sensitive profile fields are not written to normal logs.
