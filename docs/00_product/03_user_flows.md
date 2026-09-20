# User Flows

## Initial setup

Install application
→ launch CLI or desktop
→ create local candidate profile
→ import resume
→ configure LLM
→ configure job-search preferences
→ optionally configure notifications
→ ready

## Job discovery

User starts search or scheduled search
→ sources are queried
→ jobs are normalized
→ duplicates removed
→ jobs stored
→ matching runs
→ strong matches appear in history/dashboard

## Job analysis

User opens job
→ system loads normalized job
→ loads candidate context
→ LLM produces structured analysis
→ match score and explanation are persisted

## Application preparation

User selects job
→ system generates cover letter and application answers
→ content is persisted
→ user edits/reviews
→ application status becomes ready for user action

## Review mode post-MVP

Agent identifies application
→ extracts form
→ fills deterministic fields
→ generates answers for generative fields
→ pauses
→ user reviews
→ user submits

## Scout post-MVP

User configures a scout against a saved search
→ scout runs
→ agent obtains jobs (stored locally, or fetched from sources)
→ matches
→ eligibility rules evaluate each match
→ agent creates an application for each job that clears them
→ run is recorded
→ user reviews the shortlist

The scout stops there. Preparing and submitting stay in the Review flow
above, driven by the user.

## Offline

User opens app without internet
→ local history/profile/settings remain available
→ new discovery actions are unavailable or report that internet is required
