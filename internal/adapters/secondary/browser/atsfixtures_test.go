//go:build integration

package browser

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// recordATS re-records the fixtures below from their live sources. It is
// off by default: the committed pages are what the tests run against, so
// an ordinary test run touches no network and cannot fail because a job
// was filled or a board was redesigned.
//
//	go test -tags=integration ./internal/adapters/secondary/browser/ -run TestATSFixtures -record-ats
//
// Re-recording is a deliberate act with a reviewable diff. A fixture whose
// extraction changed should be read before it is committed — that diff is
// the whole point of keeping them.
var recordATS = flag.Bool("record-ats", false, "re-record the ATS fixture pages in testdata/ats from their live sources")

// updateGolden rewrites the recorded extractions from the pages already on
// disk, without touching the network. This is the flag to reach for after
// changing what we read off a page: the pages stay exactly as they were, so
// the diff is entirely ours.
//
//	go test -tags=integration ./internal/adapters/secondary/browser/ -run TestATSFixtures -update-golden
//
// Re-recording the pages implies it, since a new page needs a new golden.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/ats goldens from the pages already on disk")

// atsFixture is one saved page, and what reaching it must mean.
//
// The table states the intent — is this the form, and if not, which way in
// does it offer — while the .golden.json beside each page records the
// detail. Intent lives in code so it cannot be re-recorded away; detail
// lives in a golden file so a change in what we extract shows up as a diff
// rather than as a passing test.
type atsFixture struct {
	// name is the file base name under testdata/ats.
	name string
	// source is the live URL the page was recorded from.
	source string
	// wantForm is whether services.LooksLikeApplicationForm must accept
	// the page. The positives are employers' real application forms; the
	// negatives are the posting pages our two connectors actually hand us,
	// which must be refused however many fields they happen to carry.
	wantForm bool
	// wantApplyText and wantApplyURL are what the control PickApplyControl
	// chooses must read and point at, matched case-insensitively as
	// substrings. Both are empty for the positives, which need no way in —
	// they are the destination.
	//
	// A control can legitimately have no destination: Remotive's way in is
	// a button that navigates by script, so wantApplyURL is empty there and
	// only the wording is checked.
	wantApplyText string
	wantApplyURL  string
	// wantLabels are questions the page asks that extraction must still be
	// reading. Matched case-insensitively as substrings of a field label.
	wantLabels []string
}

// atsFixtures covers the three applicant tracking systems the boards hand
// off to, and both connectors' own posting pages as the negatives.
//
// The negatives are the ones that matter most: before ADR-0013 the worker
// scraped whatever loaded at the posting's canonical URL, found Arbeitnow's
// newsletter box, filled it with the candidate's address, and reported the
// application as prepared. Each of these pages must be refused.
var atsFixtures = []atsFixture{
	{
		name:       "ashby-application",
		source:     "https://jobs.ashbyhq.com/lemlist/a2a219b4-85de-47bc-8cbe-6f551f66e65c/application",
		wantForm:   true,
		wantLabels: []string{"name", "email"},
	},
	{
		name:       "greenhouse-application",
		source:     "https://job-boards.greenhouse.io/vercel/jobs/6136160004",
		wantForm:   true,
		wantLabels: []string{"first name", "email", "phone"},
	},
	{
		name:       "lever-application",
		source:     "https://jobs.lever.co/matchgroup/1deea0b0-c0c9-4541-972e-2a437b64b023/apply",
		wantForm:   true,
		wantLabels: []string{"name", "email", "phone"},
	},
	{
		name:          "ashby-posting",
		source:        "https://jobs.ashbyhq.com/lemlist/a2a219b4-85de-47bc-8cbe-6f551f66e65c",
		wantForm:      false,
		wantApplyText: "application",
		wantApplyURL:  "jobs.ashbyhq.com/lemlist/a2a219b4-85de-47bc-8cbe-6f551f66e65c/application",
	},
	{
		name:          "arbeitnow-posting",
		source:        "https://www.arbeitnow.com/jobs/companies/wayve/data-scientist-data-quality-provenance-214091",
		wantForm:      false,
		wantApplyText: "apply",
		wantApplyURL:  "/apply",
	},
	{
		// The way in is a button with no href, so following it is a click
		// rather than a navigation — the case a URL-only rule would miss.
		name:          "remotive-posting",
		source:        "https://remotive.com/remote-jobs/software-development/tech-lead-full-stack-rails-engineer-2069746",
		wantForm:      false,
		wantApplyText: "apply for this position",
	},
}

// goldenExtraction is the recorded shape of one fixture's extraction.
type goldenExtraction struct {
	Source       string            `json:"source"`
	IsForm       bool              `json:"is_application_form"`
	ApplyControl *goldenControl    `json:"apply_control,omitempty"`
	Fields       []goldenFormField `json:"fields"`
}

type goldenControl struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type goldenFormField struct {
	Label    string              `json:"label"`
	Kind     ports.FormFieldKind `json:"kind"`
	Required bool                `json:"required,omitempty"`
	Options  []string            `json:"options,omitempty"`
}

func atsFixtureDir() string { return filepath.Join("testdata", "ats") }

func atsPagePath(name string) string { return filepath.Join(atsFixtureDir(), name+".html") }

func atsGoldenPath(name string) string { return filepath.Join(atsFixtureDir(), name+".golden.json") }

// TestATSFixtures runs the whole route decision — extract, judge, and where
// the page is not the form, choose the way in — over saved copies of real
// applicant tracking system pages.
//
// It proves the heuristics against markup nobody on this project wrote,
// which is the only kind that can embarrass them.
func TestATSFixtures(t *testing.T) {
	if *recordATS {
		recordATSFixtures(t)
	}

	for _, fixture := range atsFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			page, err := os.ReadFile(atsPagePath(fixture.name))
			require.NoError(t, err, "fixture missing; re-record with -record-ats")

			worker := newTestWorker(t)
			require.NoError(t, worker.Navigate(t.Context(), serveATSFixture(t, page)))

			got := extractATSFixture(t, worker)

			assert.Equal(t, fixture.wantForm, got.IsForm,
				"LooksLikeApplicationForm on %s; extracted %d fields", fixture.source, len(got.Fields))

			for _, want := range fixture.wantLabels {
				assert.True(t, hasLabelContaining(got.Fields, want),
					"no extracted field asks about %q; got %v", want, labelsOf(got.Fields))
			}

			for _, field := range got.Fields {
				assert.True(t, isReadableLabel(field.Label),
					"an unreadable label reached the caller: %q", field.Label)
			}

			if !fixture.wantForm {
				require.NotNil(t, got.ApplyControl, "no way in found on a page that is not the form")
				assert.Contains(t, strings.ToLower(got.ApplyControl.Text), fixture.wantApplyText,
					"PickApplyControl chose a control reading %q", got.ApplyControl.Text)
				if fixture.wantApplyURL != "" {
					assert.Contains(t, got.ApplyControl.URL, fixture.wantApplyURL)
				}
			}

			assertGoldenExtraction(t, fixture, got)
		})
	}
}

// serveATSFixture serves one saved page, and an empty document for the
// frames the recording neutralised, so nothing in the test reaches the
// network. It returns the URL to navigate to.
func serveATSFixture(t *testing.T, page []byte) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc(offlineFramePath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><body></body></html>"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL
}

func extractATSFixture(t *testing.T, worker *PlaywrightWorker) goldenExtraction {
	t.Helper()
	ctx := t.Context()

	fields, err := worker.ExtractForm(ctx)
	require.NoError(t, err)

	got := goldenExtraction{
		IsForm: services.LooksLikeApplicationForm(fields),
		Fields: make([]goldenFormField, 0, len(fields)),
	}
	for _, field := range fields {
		got.Fields = append(got.Fields, goldenFormField{
			Label:    field.Label,
			Kind:     field.Kind,
			Required: field.Required,
			Options:  field.Options,
		})
	}

	if !got.IsForm {
		controls, err := worker.FindApplyControls(ctx)
		require.NoError(t, err)
		if picked := services.PickApplyControl(controls); picked != nil {
			got.ApplyControl = &goldenControl{Text: picked.Text, URL: picked.URL}
		}
	}
	return got
}

// assertGoldenExtraction compares what we extract now against what we
// extracted when the page was recorded. A diff here is not automatically a
// failure of the fixture — it is the question "did we mean to change what
// we read off these pages?" being asked out loud.
func assertGoldenExtraction(t *testing.T, fixture atsFixture, got goldenExtraction) {
	t.Helper()

	got.Source = fixture.source
	encoded, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')

	path := atsGoldenPath(fixture.name)
	if *recordATS || *updateGolden {
		require.NoError(t, os.WriteFile(path, encoded, 0o600))
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden missing; re-record with -record-ats")
	assert.Equal(t, string(want), string(encoded),
		"extraction changed for %s; if that was intended, re-record with -record-ats and review the diff", fixture.name)
}

func labelsOf(fields []goldenFormField) []string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, field.Label)
	}
	return labels
}

func hasLabelContaining(fields []goldenFormField, want string) bool {
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field.Label), strings.ToLower(want)) {
			return true
		}
	}
	return false
}

// offlineFramePath is where a recorded page's iframes are pointed, so the
// element survives — DetectBlocker reads CAPTCHA frames, and an Ashby form
// carries one — without the recording phoning home to its vendor.
const offlineFramePath = "/_recorded_frame"

// sanitizeRecordingScript turns the live page into a page that behaves the
// same way offline.
//
// Two things make that more than deleting <script> tags. Extraction asks
// the browser what is visible, and visibility is decided by stylesheets we
// cannot carry (they are cross-origin, so the page cannot even read them);
// so the recording freezes what the real page showed into inline styles
// first, and only then drops the stylesheets that decided it. And a control
// is judged on where it points, so every href is rewritten to the absolute
// URL it resolved to on the live site — otherwise a relative link would
// resolve against the test server and an Ashby hand-off would stop looking
// like one.
const sanitizeRecordingScript = `(framePath) => {
	document.querySelectorAll('base').forEach((el) => el.remove());
	document.querySelectorAll('a[href]').forEach((el) => {
		try { el.setAttribute('href', el.href); } catch (e) { /* leave it as written */ }
	});

	// Measure everything before changing anything: hiding one element
	// moves the others, so a second pass would measure a page that no
	// longer exists.
	const hide = [];
	const size = [];
	document.querySelectorAll('*').forEach((el) => {
		const style = getComputedStyle(el);
		const rect = el.getBoundingClientRect();
		if (style.display === 'none' || rect.width === 0 || rect.height === 0) {
			hide.push([el, 'display', 'none']);
		} else if (style.visibility === 'hidden' || style.visibility === 'collapse') {
			hide.push([el, 'visibility', 'hidden']);
		} else if (el.textContent.trim() === '') {
			// Nothing inside it to give it a box once the stylesheets are
			// gone — an icon-only apply button is exactly this shape.
			size.push([el, rect]);
		}
	});
	hide.forEach(([el, property, value]) => el.style.setProperty(property, value));
	size.forEach(([el, rect]) => {
		el.style.setProperty('min-width', rect.width + 'px');
		el.style.setProperty('min-height', rect.height + 'px');
	});

	document.querySelectorAll('iframe').forEach((el) => {
		// Keep the vendor's name in the URL: it is what DetectBlocker
		// matches on, and whether an Ashby form's invisible reCAPTCHA is
		// read as a challenge is precisely what these pages can answer.
		el.setAttribute('src', framePath + '?src=' + encodeURIComponent(el.getAttribute('src') || ''));
	});
	document.querySelectorAll('img').forEach((el) => {
		el.removeAttribute('src');
		el.removeAttribute('srcset');
	});
	document.querySelectorAll('script, noscript, link, style, svg, source, video, audio, object, embed')
		.forEach((el) => el.remove());

	return '<!doctype html>\n' + document.documentElement.outerHTML + '\n';
}`

// recordATSFixtures rewrites every page in testdata/ats from its live
// source. See recordATS for why this is not part of an ordinary run.
func recordATSFixtures(t *testing.T) {
	t.Helper()

	worker := newTestWorker(t)
	for _, fixture := range atsFixtures {
		require.NoError(t, worker.Navigate(t.Context(), fixture.source), "recording %s", fixture.name)
		waitForRecordingToSettle(t, worker)

		result, err := worker.page.Evaluate(sanitizeRecordingScript, offlineFramePath)
		require.NoError(t, err, "recording %s", fixture.name)
		html, ok := result.(string)
		require.True(t, ok, "recording %s: sanitizer returned %T", fixture.name, result)

		require.NoError(t, os.WriteFile(atsPagePath(fixture.name), []byte(html), 0o600))
		t.Logf("recorded %s from %s (%d KiB)", fixture.name, fixture.source, len(html)/1024)
	}
}

// waitForRecordingToSettle gives a page that builds its form in the browser
// — every one of these ATS pages does — the chance to finish before it is
// photographed. A page that never goes quiet is still worth recording, so a
// timeout here is not an error.
func waitForRecordingToSettle(t *testing.T, worker *PlaywrightWorker) {
	t.Helper()
	_ = worker.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(30_000),
	})
	_ = worker.page.Locator("input, select, textarea").First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateAttached,
		Timeout: playwright.Float(15_000),
	})
}
