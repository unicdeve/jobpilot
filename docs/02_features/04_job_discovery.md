# Feature: Job Discovery

## Purpose

Find jobs using configured criteria and supported sources.

## Search criteria

- Keywords
- Job title
- Location
- Remote preference
- Employment type
- Salary preference
- Experience preference
- Included sources
- Excluded companies
- Optional custom URLs

## Source strategy

Discovery is connector-driven, not scraper-first.

Initial public source implementations should prioritize stable APIs/feeds/public ATS pages where practical.

## Acceptance Criteria

### AC-001
A user can save search criteria.

### AC-002
A search can be executed from the CLI.

### AC-003
A search can be executed from the desktop application.

### AC-004
Jobs from supported sources are normalized into the common Job schema.

### AC-005
Duplicate jobs are not created repeatedly for the same canonical job.

### AC-006
When internet access is unavailable, discovery fails clearly without corrupting existing history.

### AC-007
Custom job URLs can be submitted for individual job analysis.

### AC-008
Source failures do not prevent successful results from other sources.
