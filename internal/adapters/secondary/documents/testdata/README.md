# Document converter fixtures

`resume.pdf` is a synthetic one-page resume.
No part of it belongs to a real person: a real resume cannot be committed here, and a redacted one would no longer provoke the defect it exists to prove.

It is not an arbitrary sample.
It was written to reproduce the failure ADR-0016 describes, and `TestPDFFixtureStillProvokesTheDefect` asserts that it still does — that Go MarkItDown's rectangle-based extraction mangles it into `Ada Ll` / `oveace`, `adalovelacedev@examplecom` and `SUMMARYRY`.
Without that assertion the golden test would pass whether or not the fix was present, which is the usual way a regression test quietly stops being one.

It also carries link annotations — the candidate's own, an employer's, and an open-source project's — because a URL in a PDF is an annotation rather than text, and telling those three apart is what stops a project link being extracted as the candidate's GitHub profile.

## Regenerating

`resume.html` is the source. Print it with headless Chrome:

```sh
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless --disable-gpu --no-pdf-header-footer \
  --print-to-pdf=internal/adapters/secondary/documents/testdata/resume.pdf \
  "file://$PWD/internal/adapters/secondary/documents/testdata/resume.html"

go test ./internal/adapters/secondary/documents -update-golden
```

The corruption comes from glyph geometry, so a different renderer, font or page size may not provoke it.
If `TestPDFFixtureStillProvokesTheDefect` fails after regenerating, the new PDF is not a usable fixture — keep the old one rather than weakening the test.
