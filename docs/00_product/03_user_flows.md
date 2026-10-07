# User Flows

These flows describe the implemented product.
See [current state](current_state.md) for limitations and platform verification.

## Initial setup

Launch CLI or desktop
→ initialize local storage
→ configure LLM
→ create profile, optionally prefilling from an imported resume
→ optionally import a resume
→ configure at least one saved search
→ ready

The desktop queues its newly added searches when setup finishes.
The CLI prints the command to run them.
Offline discovery failures do not undo completed setup.

## Discovery and analysis

Choose a saved search
→ query supported sources and watched company pages
→ normalize and deduplicate
→ store jobs
→ report new jobs and source failures
→ user opens a job
→ user requests analysis
→ load candidate context
→ validate and persist the model's score and explanation

Discovery alone does not analyze every result.
A Scout combines discovery and analysis; browsing stored jobs is a separate local operation.
Custom job URLs can also be imported and analyzed.

## Application content

User selects a job
→ creates an application and optionally chooses a resume
→ adds personal context
→ generates a letter or resolves questions
→ reviews draft flags and edits content
→ prepares for review

Cover-letter versions and application transitions remain in local history.
Unknown factual answers are left for the user.

## Review and browser handoff

User asks to prepare a Review application in the browser
→ open posting in JobPilot's Chrome profile
→ find and verify an application form
→ extract questions and resolve supported answers
→ fill resolved fields
→ leave the browser open with unresolved fields blank
→ user reviews and submits in the browser
→ user records approval/submission and subsequent outcomes in JobPilot

JobPilot never clicks the external submit control.
Approval and submission tracking are local records, not commands to an employer.
A detected blocker or preparation failure stops automation and leaves the browser open for the user.
Assist applications do not enter this browser flow.

## Scouts

User configures a Scout over a saved search
→ runs it manually or enables an OS schedule
→ Scout obtains local or freshly discovered jobs
→ analyzes eligible jobs
→ deterministic rules evaluate score and per-run limits
→ creates applications for review
→ records decisions in Activity and eligible notifications
→ user reviews the shortlist

A Scout stops before browser preparation and never contacts an employer.

## Offline

User opens the app without internet
→ local history, profile and settings remain available
→ fresh discovery reports that internet is required
→ model work is available only if the configured model can be reached
