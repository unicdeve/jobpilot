# Feature: Resume Management

## Purpose

Import and manage resume documents locally.

## Requirements

- Import supported document formats, initially PDF.
- Convert PDF/document content into Markdown where supported.
- Preserve original document.
- Store parsed Markdown locally.
- Associate resumes with candidate profile.
- Allow deletion.
- After import, the LLM may propose a candidate Summary and Skills list
  derived from the resume; the user must review and can edit each field
  before it is written to the profile. A failure to generate suggestions
  does not affect the already-imported resume.

## Acceptance Criteria

### AC-001
A supported PDF can be imported and retained locally.

### AC-002
The system produces Markdown when parsing succeeds.

### AC-003
Parsing failure does not delete the original document.

### AC-004
User can delete a resume and its derived content according to retention rules.

### AC-005
Resume processing does not require a cloud service.

### AC-006
After a successful import, the user is shown an LLM-proposed Summary and
Skills draft derived from the resume and can accept, edit, or reject each
field before it is written to the candidate profile. Nothing is written to
the profile without this review step.

### AC-007
If suggestion generation fails (e.g. the LLM is unavailable), the resume
import itself still succeeds; only a warning is shown.
