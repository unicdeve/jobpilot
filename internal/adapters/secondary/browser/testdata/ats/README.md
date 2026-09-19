# Recorded applicant tracking system pages

Saved copies of real pages the browser worker has to get right, and what it
extracted from each when they were saved.
They are driven by `TestATSFixtures` in `../../atsfixtures_test.go`.

Each fixture is a pair:

- `<name>.html` — the page, as rendered, recorded from the URL in the golden's `source` field.
- `<name>.golden.json` — what `ExtractForm`, `LooksLikeApplicationForm` and `PickApplyControl` made of it.

## What they are for

Three of them are employers' real application forms, on the three systems job boards hand off to — Ashby, Greenhouse and Lever.
Three are the posting pages our own connectors hand us, which are not forms and must be refused.

The negatives are the ones that earn their keep.
Before ADR-0013 the worker scraped whatever loaded at a posting's canonical URL, found Arbeitnow's newsletter box, filled it with the candidate's email address, and reported the application as prepared.
`arbeitnow-posting.html` is that page.

These are regression tests, not a monitor.
A saved page keeps passing after the live site changes; what it catches is our own extraction quietly getting worse, on markup nobody here wrote.

## Why the saved pages behave the way the live ones did

Extraction asks the browser what is *visible*, and a page's visibility is decided by stylesheets we cannot carry — they are cross-origin, so the page cannot even read them to inline them.
So the recording measures the live page first and freezes the answer into inline styles, then drops the stylesheets that decided it.
An element that was hidden stays hidden; an element that was visible but had no text to give it a box — an icon-only apply button is exactly this shape — keeps its measured size.

Two other things are rewritten at recording time:

- Every `href` becomes the absolute URL it resolved to on the live site. `PickApplyControl` judges a control by where it points, so a relative link left alone would resolve against the test server and an Ashby hand-off would stop looking like one.
- Every `iframe` src is pointed at the test server, keeping the vendor's name in the URL. `DetectBlocker` matches on that name, and whether an Ashby form's invisible reCAPTCHA is read as a challenge is one of the things these pages can answer.

Scripts, stylesheets, images and media are removed outright.
Nothing in a test run reaches the network.

## Re-recording

```sh
# Fetch the pages again and rewrite the goldens. Needs network and Chrome.
go test -tags=integration ./internal/adapters/secondary/browser/ -run TestATSFixtures -record-ats

# Rewrite the goldens only, from the pages already here. No network.
go test -tags=integration ./internal/adapters/secondary/browser/ -run TestATSFixtures -update-golden
```

Reach for `-update-golden` after changing what we read off a page: the pages stay byte-for-byte as they were, so the whole diff is ours to read.

Reach for `-record-ats` only when a fixture has genuinely rotted — a job was filled, a board was redesigned.
It will pick up whatever those URLs serve that day, so the `source` URLs in the table may need updating first, and the resulting diff needs reading rather than accepting.
