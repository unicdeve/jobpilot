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
- cover_letter
- questions[]
- policy_result
- approval_status
- submitted_at
- failure_reason
- created_at
- updated_at
```

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
- confidence
- user_edited
- status
```

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
