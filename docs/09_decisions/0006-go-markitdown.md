# ADR-0006: Go MarkItDown

## Status

Accepted.

Narrowed by [ADR-0016](0016-reading-pdf-resumes.md), which carves PDF out of this decision.
Go MarkItDown's PDF converter reads text from PDFium text rectangles, which both duplicates and deletes characters; measured on a real resume it agrees with a reference extraction on 61% of tokens, against 96% for the plain-text path of the same engine.
Every other format this decision covers is unaffected, and the rationale below is not weakened by that change.

## Decision

Use the Go MarkItDown implementation for document-to-Markdown conversion where its supported formats meet requirements.

## Rationale

Avoid an unnecessary Python runtime for document conversion.
