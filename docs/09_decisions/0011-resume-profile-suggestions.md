# ADR-0011: Resume-Derived Profile Suggestions Are Review-Gated

## Status

Accepted

## Decision

After a resume is imported, the LLM may propose a candidate Summary and
Skills draft derived from the resume content. The draft is never written to
the candidate profile automatically: the user must review it and can accept
or edit each field individually before it is persisted, using the same
review UI as manual profile editing. If suggestion generation fails (LLM
unavailable, unparsable response, etc.), the already-completed resume import
is unaffected — only a warning is shown.

## Rationale

The candidate profile is factual source material (`docs/02_features/02_candidate_profile.md`)
and `AGENTS.md` requires that candidate facts never be invented and that the
LLM never bypass explicit user review. Auto-populating Summary/Skills
directly from an LLM's reading of a resume would violate both principles.
Gating the suggestion behind the same review step already used for manual
profile edits keeps behavior consistent and closes the gap between the data
model (`Candidate.Summary`/`Skills`, `Resume.ParsedMetadata`) and its actual
behavior without introducing a new UI paradigm.
