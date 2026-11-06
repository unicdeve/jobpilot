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

## Applying

User selects a job
→ creates an application and optionally chooses a resume
→ opens the application page in their own browser
→ copies their profile details and own answers into the form
→ writes their cover letter in JobPilot, reading its draft flags
→ notes the form's questions and writes their answers, using a suggestion from their profile or a kept answer where one fits
→ submits on the employer's site
→ records that they sent it, and later what happened

JobPilot never writes the letter or an answer, and never touches the employer's form (ADR-0034).
Cover-letter versions, questions and application transitions remain in local history.
Marking an application ready to send and recording submission are local records, not commands to an employer.

## Scouts

User configures a Scout over a saved search
→ runs it manually or enables an OS schedule
→ Scout obtains local or freshly discovered jobs
→ analyzes eligible jobs
→ deterministic rules evaluate score and per-run limits
→ creates applications for the candidate
→ records decisions in Activity and eligible notifications
→ user reviews the shortlist and applies to the ones they want

A Scout never contacts an employer.

## Offline

User opens the app without internet
→ local history, profile and settings remain available
→ fresh discovery reports that internet is required
→ model work is available only if the configured model can be reached
