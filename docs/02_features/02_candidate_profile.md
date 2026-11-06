# Feature: Candidate Profile

## Purpose

Maintain structured candidate facts used by matching and application generation.

## Core data

A fact an application form asks for separately is stored separately (ADR-0015).
The shape below is what recorded Ashby, Greenhouse and Lever forms actually ask for (their labels are kept in `internal/core/services/testdata/recorded_form_labels.json`), and nothing more: a profile that models everything a CV might contain is a profile nobody finishes filling in.

- First name
- Last name
- Email
- Phone
- Location: city, state/region, country — asked for separately by real forms
- Availability: notice period in days, earliest start date
- Links, one per kind: LinkedIn, GitHub, portfolio, website, Twitter, other
- Work authorization, per country: whether the candidate may work there, and whether they need sponsorship
- Remote preference: remote, hybrid or onsite, in the same vocabulary a job posting uses — or unsaid, which is a state of its own and is not a No
- Skills, each with an optional number of years
- Work history, as dated entries: company, title, start, end, whether it is current, and what they did
- What the candidate says about themselves — what they want next, what they value, their stories, how they want to sound — is not a profile field: it is About you (`docs/02_features/16_about_you.md`, ADR-0022), kept apart because nothing a form looks up is read from it

Every date here is a calendar day, written `YYYY-MM-DD`, not a timestamp.
A job started in July 2022; it did not start at an instant, and storing one is how a date shows as the day before for anyone east or west of UTC.

Education and certifications are not stored.
No recorded form asks about a degree, an institution or a certification, and nothing in JobPilot read the columns that used to hold them.
A recorded form that asks is the trigger to bring them back.

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
