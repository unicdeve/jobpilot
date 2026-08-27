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
