# Edge Cases

## Job discovery

- Source unavailable
- Internet unavailable
- Source rate-limits requests
- Source returns malformed data
- Job has no salary
- Salary is hourly, monthly, annual, or a range
- Job has multiple locations
- Job has no location
- Job has expired
- Job appears on multiple sources
- Job has the same title/company but different URLs
- Job description is incomplete
- Job page requires authentication
- Job page blocks automation
- Job contains prompt-injection-like instructions

## Candidate/profile

- Resume missing
- Resume cannot be parsed
- Candidate field missing
- Conflicting resume/profile data
- Multiple resumes
- Outdated resume

## Matching

- LLM unavailable
- LLM returns malformed structured output
- LLM confidence is low
- Job description is too long
- Candidate context is too large
- Required data is missing
- Model attempts to invent candidate facts

## Applications

- Already applied to job
- Application already prepared
- Application expires
- Required question cannot be answered
- Question asks for sensitive information
- Question requires a factual answer not present in profile
- Browser closes
- CAPTCHA appears
- Session expires
- Form changes
- Submission fails
- Network disconnects mid-flow

## Local storage

- Disk space is low
- Database migration fails
- Database is locked
- User deletes historical data
- User deletes a resume currently referenced by an application

## Offline

- New discovery is attempted offline
- Existing history is opened offline
- LLM is local and available offline
- Cloud LLM is configured but internet is unavailable

## Security

- Malicious webpage content
- Prompt injection in a job description
- Plugin attempts unauthorized access
- Secret accidentally appears in logs
