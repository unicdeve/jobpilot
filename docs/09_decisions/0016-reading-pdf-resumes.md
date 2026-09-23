# ADR-0016: Reading a PDF resume correctly

## Status

Accepted, and implemented in full.

Both open questions below were settled during implementation: the fixture could be built, and stored resumes are not re-converted.

This narrows ADR-0006 rather than replacing it.
Go MarkItDown stays the converter for every format JobPilot handles except PDF, and the reason ADR-0006 gave for choosing it — no Python runtime — is not weakened by anything here.

## Context

Resume import is the front door of the product.
Everything downstream reads its output: the profile draft the user reviews, the matcher, the cover letter, and the answers typed into an employer's form.

Measured on a real two-page resume exported from Google Docs (`bin/Taiwo_resume.pdf`, September 2026), `markitdown.New().Convert` returns this:

```
# TaiOl

# Tawo gunoa

taiwoogunoladev@gmailcom | Remote | Github ub | LinkedIn | Blog

#### PROFESSIONAL SUMMARYRY

Produduct Engineer with over 6 years of experience who ships quickly and owns systems endtoendSkilled in leveraging AI tools like
Cursor and Claude to enhance coding effffificiency, streamline debuggingand optimize workflflowswhile validating AI outptputs fofor
```

The candidate's name, their email address and roughly a third of every sentence are wrong.
Note that the damage runs in both directions at once: characters are duplicated (`Produduct`, `effffificiency`, `SUMMARYRY`) and characters are deleted (`Tawo gunoa`, and every `.`, `,` and `-` in the paragraph).

Quantified against `pdftotext` as a reference, counting whitespace-separated tokens across both pages:

| extraction | tokens matching the reference | of 1071 |
| --- | --- | --- |
| markitdown v0.0.1 | 649 | 61% |
| PDFium plain page text | 1027 | 96% |

The corrupted output is also *longer* than the correct one — 8676 bytes against 7907 — which is what duplication and deletion happening together looks like.

### This is not the engine, and not the fonts

The obvious suspect was the PDF: Google Docs exports Type0 CID fonts with ToUnicode CMaps, and those are a classic source of mojibake.
That suspicion is wrong, and the measurement above is what rules it out.

`converter_pdf_pdfium.go` calls `GetPageTextStructured` in `ModeRects`, then concatenates the `text` of each rectangle in reading order.
PDFium's rect API returns bounded text for overlapping regions, so a character that falls inside two rectangles is emitted twice, and a narrow glyph — `i`, `l`, `.`, `,`, `-` — whose box lands between two rectangles is emitted by neither.
Both symptoms follow from that one decision, and neither involves character encoding at all.

The decisive evidence is that the same library already contains the correct path.
`extractPlainPage` calls `GetPageText`, and it is reached only as a fallback when the structured call errors or returns no rectangles.
Driving that call directly — same library, same PDFium WebAssembly build, same file — produces text that matches `pdftotext`:

```
Taiwo Ogunola
taiwo.ogunola.dev@gmail.com | Remote | Github | LinkedIn | Blog
PROFESSIONAL SUMMARY
Product Engineer with over 6 years of experience who ships quickly and owns systems end-to-end. Skilled in leveraging AI tools like
Cursor and Claude to enhance coding efficiency, streamline debugging, and optimize workflows, while validating AI outputs for
```

The defect is confined to PDF.
`converter_pdf_pdfium.go` is the only converter in the library that uses PDFium or geometry at all; DOCX reads `word/document.xml` directly, and HTML parses markup, so neither can fail this way.

### Why this matters more than it looks

ADR-0015 §7 dropped the profile's prose blobs and had the resume repopulate the structured fields instead.
Every guard written there — `NormalizeDraftURL` rejecting `ub` and `Blog`, the refusal to invent work authorization, the refusal to estimate a skill's duration — was built on top of an input that was already 39% wrong.
Those guards catch a value that is obviously not a URL.
They cannot catch `taiwoogunoladev@gmailcom`, which is a plausible-looking email address, is wrong, and would be typed into an employer's form under the candidate's name.

That is the failure mode AGENTS.md principle 6 exists to prevent, and it is currently reachable from the product's first screen.

### Links are annotations, not text

A separate finding from the same measurement, and the explanation for a defect ADR-0015 only defended against.

In a PDF, a hyperlink is an annotation with a rectangle, not text.
`Github`, `LinkedIn` and `Blog` are the visible labels; the URLs are not in the content stream at all.
So `pdftotext` loses them, the plain PDFium path loses them, and the model — asked for the candidate's GitHub URL and given only the word `Github` — proposed `ub`.

They are recoverable.
`FPDFLink_Enumerate` with `FPDFAction_GetURIPath` on the same document returns, among others:

```
mailto:taiwo.ogunola.dev@gmail.com
https://github.com/unicdeve
https://www.linkedin.com/in/ogunola-taiwo
https://unicdev.hashnode.dev/
```

Those are exactly the four facts the extraction was guessing at.

### A second problem, in core

`markitdown` is imported by `internal/core/services/resume.go` and `internal/core/services/discovery.go`.
Core constructs the converter itself, so a decision about a document-conversion library is currently an edit inside the hexagon, and neither service can be tested without running a real PDF engine.
`internal/adapters/secondary/discovery/htmlconvert.go` does the same conversion from the correct side of the boundary, which shows what the shape should have been.

## Decision drivers

- Never fabricate candidate facts (AGENTS.md principle 6). A silently mangled email is a fabricated fact.
- No Python runtime, and no new external binary to install alongside the app. This is ADR-0006's actual rationale and it still holds.
- Core logic must be testable independently from external services (AGENTS.md principle 14).
- Avoid infrastructure not justified by a documented requirement (principle 10) — including our own code.
- An accepted decision must not be changed silently.

## Options

### A. Fix it upstream and wait

Correct in principle, and the fix is small — prefer `GetPageText` over the rect path, or de-duplicate the rect overlaps.

The library is at `v0.0.1`.
We control neither review nor release, and the product is broken at its front door today.
Waiting is not a plan, though sending the patch is still worth doing.

### B. Fork or vendor Go MarkItDown

Takes ownership of ten converters to fix one, and every future upstream improvement becomes a merge.
Disproportionate to a defect confined to a single file.

### C. Replace it with poppler / `pdftotext`

The reference implementation in the measurement above, and the most accurate option available.

It is an external native binary the user must install and we must locate at runtime, on three platforms.
That is the same objection ADR-0006 raised against a Python runtime, arriving by a different door.

### D. A JobPilot PDF converter behind a port, on the PDFium we already ship

Go MarkItDown keeps every other format.
PDF goes through the plain page text of the same PDFium WebAssembly build the library already pulls in, so the dependency set does not grow.

The measured cost of this is known rather than assumed: it buys 61% → 96% fidelity and costs the heading structure discussed under Consequences.

## Decision

Option D, in four parts.

### 1. Document conversion becomes a port

Add `ports.DocumentConverter` with the two conversions the product performs:

```go
type DocumentConverter interface {
    ConvertFile(ctx context.Context, path string) (Document, error)
    ConvertHTML(ctx context.Context, html string) (Document, error)
}
```

`Document` carries the markdown.
It is a struct rather than a string so that what a converter recovers can grow without changing every caller; recovered links are folded into the markdown, since the markdown is what is stored and what the model reads, and nothing today needs them separately.

`ResumeService` and the discovery service take the port instead of importing `markitdown`.
The adapter lives at `internal/adapters/secondary/documents`, alongside the other secondary adapters, and `discovery/htmlconvert.go` collapses into it.
Core stops depending on a conversion library, which is the arrangement the architecture already assumed.

### 2. PDF text comes from the page, not from rectangles

The adapter extracts PDF text with PDFium's `GetPageText` per page.
It does not use `GetPageTextStructured`, and it does not reimplement the rect path with the overlaps fixed.

Correct characters are the requirement; layout is not.

### 3. Link annotations are recovered, with the text they sit on

The adapter enumerates each page's link annotations and resolves their URI actions.
Recovered URLs are appended to the converted document under a heading, so the model reading a resume sees the real `https://github.com/unicdeve` rather than the word `Github`.

Each URL is paired with its anchor text, which is a correction made during implementation rather than in the original draft.
A bare list of URLs turned out to make the problem worse, not better.
Measured on the same resume, it recovers twenty-one links, of which four are the candidate's and the rest belong to employers, to an open-source project they contributed to, and to course certificates.
Handing the model `github.com/ThallesP/nestjs-better-auth` with no context invites it to propose that as the candidate's GitHub profile — a plausible, wrong, fabricated fact, which is the exact failure this ADR exists to stop rather than relocate.
`Github -> https://github.com/unicdeve` says whose it is.

Anchors come from PDFium's per-character positions intersected with each annotation's rectangle.
Characters are one-to-one with the text page, so the duplication and deletion that made the rectangle path unusable cannot occur in this path either.

They remain candidate-supplied values from an untrusted document, so they stay subject to `NormalizeDraftURL` and to the user's review of the draft.
This removes the guesswork, not the review.

### 4. Everything else stays with Go MarkItDown

DOCX, HTML, plain text, and the rest keep the existing converter, and ADR-0006 stands with PDF carved out.
AGENTS.md principle 13 is amended to say so.

The upstream fix is offered to `conductor-oss/markitdown` regardless.
If it lands and ships, this ADR is worth revisiting; until then the product does not depend on that happening.

## Consequences

- Resume import goes from 61% to roughly 96% agreement with the reference extraction, and the candidate's name and email address survive it.
- The four links on the measured resume are recovered exactly instead of guessed, which retires the defect `NormalizeDraftURL` was written to contain. That guard stays, as a second line of defence rather than the only one.
- **A real loss: PDFs stop getting heading structure.** The rect path inferred `#` levels by comparing font sizes, and the plain path returns flat text. The markdown will be less pretty. It will also be *right*, and an LLM reads correct prose better than it reads corrupted headings. Recovering headings from rect *geometry* while taking the *text* from the plain path is a clean future refinement, and is deliberately not in this change.
- Core gains a port and loses two direct library imports, so `ResumeService` becomes testable with a fake converter.
- We own about a hundred lines of PDF extraction. The dependency set does not change: `go-pdfium` is already pulled in transitively, and its WebAssembly build means there is still nothing for the user to install.
- **Resumes already imported keep their corrupted markdown.** Nothing rewrites stored rows. The user re-imports, which for a local product with one user today is the proportionate answer; see the open question below.

### Verification

The fixture discipline follows the ATS goldens: a committed sample PDF and a golden of its conversion, at `internal/adapters/secondary/documents/testdata/`.

`TestPDFFixtureStillProvokesTheDefect` asserts the inverse of everything else in that file — that Go MarkItDown's rectangle path really does mangle this fixture.
Without it the golden test would pass whether or not the fix were present, which is the usual way a regression test quietly stops being one.

### Found while measuring, not fixed here

The desktop's resume picker (`desktop/src/pages/resumes.tsx:23`) calls `open()` with no extension filter, so the user can choose any file at all and discover the problem from a conversion error.
That is a UI defect worth its own small change, not part of this decision.

## Settled during implementation

1. **The fixture could be built.** The open question was whether a redistributable PDF would provoke a defect that depends on glyph geometry. A one-page synthetic resume printed from HTML by headless Chrome does, and with the same signature as the real document: `Ada Ll` / `oveace`, `adalovelacedev@examplecom`, `SUMMARYRY`. No fallback to a weaker local-only check was needed. The generator HTML is committed beside it, and the README warns that a different renderer or font may not reproduce the defect.
2. **Stored resumes are not re-converted.** Decided by the user. A migration could not do it in any case — conversion needs the original file, and `original_file_path` may no longer resolve — and re-importing is a few seconds' work for the one local user who has one.
