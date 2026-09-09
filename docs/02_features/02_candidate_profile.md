# Feature: Candidate Profile

## Purpose

Maintain structured candidate facts used by matching and application generation.

## Core data

- First name
- Last name
- Contact information
- Location
- Work preferences
- Skills
- Experience
- Education
- Certifications
- Portfolio/links
- Work authorization information where the user chooses to store it
- Salary preferences
- Application preferences

## Rules

Candidate profile data is factual source material. The LLM must not invent missing values.

Summary and Skills may be populated from an LLM-generated draft after resume
import (see `03_resume_management.md`), but only through the same explicit
review/edit step as manual entry — the LLM never writes to the profile
directly.

## Acceptance Criteria

### AC-001
User can create and edit a profile locally.

### AC-002
Profile data survives application restart.

### AC-003
Profile fields can be deleted or cleared.

### AC-004
Generated application content can reference profile facts but cannot silently modify the profile.

### AC-005
Sensitive profile fields are not written to normal logs.
