# Data Schemas

The following are conceptual schemas. The implementation may choose idiomatic Go/database representations.

## Job

```text
Job
- id
- canonical_url
- source_id
- source_job_id
- title
- company_name
- company_url
- description_markdown
- locations[]
- remote_type
- employment_type
- salary_min
- salary_max
- salary_currency
- salary_period
- experience_min
- experience_max
- skills[]
- posted_at
- expires_at
- discovered_at
- updated_at
- raw_source_reference
```

## Candidate

```text
Candidate
- id
- name
- contact
- location
- summary
- work_preferences
- salary_preferences
- work_authorization
- skills[]
- experiences[]
- education[]
- certifications[]
- links[]
- created_at
- updated_at
```

## Resume

```text
Resume
- id
- candidate_id
- name
- original_file_path
- markdown_content
- parsed_metadata
- created_at
- updated_at
```

## JobMatch

```text
JobMatch
- id
- job_id
- candidate_id
- score
- confidence
- strengths[]
- gaps[]
- concerns[]
- matched_skills[]
- missing_skills[]
- rationale
- model_provider
- model_name
- created_at
```

## Application

```text
Application
- id
- job_id
- candidate_id
- resume_id
- mode
- state
- current_cover_letter_id
- questions[]
- policy_result
- approval_status
- submitted_at
- failure_reason
- created_at
- updated_at
```

`current_cover_letter_id` references the Application's current CoverLetter.
Cover letters are versioned (see below) rather than stored inline, so
regenerating one never discards the prior attempt.

## ApplicationQuestion

```text
ApplicationQuestion
- id
- application_id
- label
- type
- required
- answer
- answer_source
- category
- confidence
- user_edited
- status
```

`category` records which of the four answer-resolver categories in
`docs/02_features/07_application_answers.md` produced the answer
(`profile_fact | derived | generated | needs_user_input`); `answer_source`
holds free-text detail (e.g. which profile field, or which derivation rule).

## CoverLetter

```text
CoverLetter
- id
- application_id
- content
- model_provider
- model_name
- user_edited
- created_at
```

Each regeneration creates a new row rather than overwriting the previous
one, satisfying the requirement that generated content be stored in
application history (`docs/02_features/06_cover_letters.md` AC-005).

## ApplicationStateTransition

```text
ApplicationStateTransition
- id
- application_id
- from_state
- to_state
- reason
- created_at
```

Durable history of an Application's state changes, independent of its
current mutable `state` field (`docs/02_features/08_applications.md`
AC-003).

## AgentRun

```text
AgentRun
- id
- task_type
- status
- started_at
- completed_at
- error
```

## AgentEvent

```text
AgentEvent
- id
- run_id
- action
- tool
- status
- input_summary
- output_summary
- policy_result
- duration_ms
- created_at
```
