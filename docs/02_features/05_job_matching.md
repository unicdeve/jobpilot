# Feature: Job Matching

## Purpose

Evaluate how well a job matches the candidate profile and resume.

## Output

- score: 0-100
- strengths
- gaps
- concerns
- matched skills
- missing skills
- concise rationale
- confidence

## Rules

- The model must not invent candidate experience.
- Deterministic facts should be sourced from the candidate profile.
- The output must be validated.
- Low-confidence analysis should be clearly marked.

## Acceptance Criteria

### AC-001
Every analyzed job receives a score between 0 and 100.

### AC-002
Every analysis contains structured strengths and gaps.

### AC-003
Missing candidate information is represented as unknown rather than fabricated.

### AC-004
Malformed LLM output is rejected or safely repaired through a controlled retry strategy.

### AC-005
The analysis is persisted locally.

### AC-006
The user can inspect the analysis from CLI and desktop.
