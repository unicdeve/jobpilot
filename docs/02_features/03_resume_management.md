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
- After import, the LLM may propose candidate profile fields (first/last
  name, contact, location, Summary, Skills) derived from the resume; the
  user must review and can edit each field before it is written to the
  profile. A failure to generate suggestions does not affect the
  already-imported resume.
- The desktop Profile screen can import a resume directly to prefill the
  profile form with these same proposed fields, before the user saves —
  this is a second entry point onto the same review-gated draft mechanism
  the CLI onboarding flow uses, not a separate feature.

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
After a successful import, the user is shown an LLM-proposed draft (first/
last name, contact, location, Summary, Skills) derived from the resume and
can accept, edit, or reject each field before it is written to the
candidate profile. Nothing is written to the profile without this review
step. A field the LLM can't find in the resume is left blank/empty rather
than guessed.

In the CLI, each field is prompted individually, defaulting to the
existing profile value (or the draft value if the field was previously
empty) so pressing Enter never blanks out an existing value.

In the desktop app, imported draft fields are merged into the in-progress
form state, which the user still reviews and edits before saving. An
"Overwrite existing fields with resume data" toggle (default off)
controls the merge: with the toggle off, a draft field is only applied
when the corresponding form field is currently empty, so it never
overwrites a value already typed or loaded from a saved profile; with the
toggle on, a non-empty draft field always overwrites the current value. In
both modes, an empty draft field never changes the current field.

### AC-007
If suggestion generation fails (e.g. the LLM is unavailable), the resume
import itself still succeeds; only a warning is shown.
