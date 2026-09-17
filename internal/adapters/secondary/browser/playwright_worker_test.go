//go:build integration

package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fixtureFormHTML is a small application-form-shaped page covering every
// field kind ExtractForm/Fill need to handle: a labelled text input, a
// textarea, a select, a checkbox, and a fieldset/legend radio group — plus
// a hidden input and a submit button that must be skipped.
const fixtureFormHTML = `<!doctype html>
<html>
<body>
<form>
	<label for="full-name">Full name</label>
	<input id="full-name" name="full_name" type="text" required>

	<label for="cover-note">Why do you want to work here?</label>
	<textarea id="cover-note" name="cover_note"></textarea>

	<label for="country">Country</label>
	<select id="country" name="country">
		<option>United States</option>
		<option>United Kingdom</option>
		<option>Nigeria</option>
	</select>

	<label><input type="checkbox" name="agree"> I agree to the terms</label>

	<fieldset>
		<legend>Are you authorized to work in this country?</legend>
		<label><input type="radio" name="authorized" value="yes"> Yes</label>
		<label><input type="radio" name="authorized" value="no"> No</label>
	</fieldset>

	<input type="hidden" name="source" value="jobpilot-test">
	<button type="submit">Submit application</button>
</form>
</body>
</html>`

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixtureFormHTML))
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestSession(t *testing.T) *ChromeSession {
	t.Helper()
	session := NewChromeSession(t.TempDir(), true)
	t.Cleanup(session.Shutdown)
	return session
}

func newTestWorker(t *testing.T) *PlaywrightWorker {
	t.Helper()
	worker, err := newTestSession(t).NewWorker()
	if err != nil {
		t.Skipf("skipping: real Chrome/Playwright driver not available in this environment: %v", err)
	}
	// The concrete type, so assertions can read the live page back directly.
	return worker.(*PlaywrightWorker)
}

func TestPlaywrightWorker_ExtractAndFillForm(t *testing.T) {
	server := newFixtureServer(t)
	worker := newTestWorker(t)
	ctx := context.Background()

	require.NoError(t, worker.Navigate(ctx, server.URL))

	reason, blocked := worker.DetectBlocker(ctx)
	assert.False(t, blocked, "an ordinary form page must never be flagged as blocked: %s", reason)

	fields, err := worker.ExtractForm(ctx)
	require.NoError(t, err)

	byLabel := map[string]ports.FormField{}
	for _, f := range fields {
		byLabel[f.Label] = f
	}

	// Hidden input and submit button must be skipped, leaving exactly the
	// five real fields (the radio pair collapses into one grouped field).
	require.Len(t, fields, 5)

	name, ok := byLabel["Full name"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindText, name.Kind)
	assert.True(t, name.Required)

	note, ok := byLabel["Why do you want to work here?"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindTextarea, note.Kind)

	country, ok := byLabel["Country"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindSelect, country.Kind)
	assert.Equal(t, []string{"United States", "United Kingdom", "Nigeria"}, country.Options)

	agree, ok := byLabel["I agree to the terms"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindCheckbox, agree.Kind)

	authorized, ok := byLabel["Are you authorized to work in this country?"]
	require.True(t, ok)
	assert.Equal(t, ports.FormFieldKindRadio, authorized.Kind)
	assert.Equal(t, []string{"Yes", "No"}, authorized.Options)

	// Fill every field and verify the value actually landed, reading the
	// live page back directly (this test lives in the same package, so
	// the worker's private page field is reachable).
	require.NoError(t, worker.Fill(ctx, name.Ref, "Ada Lovelace"))
	got, err := worker.page.Locator("#full-name").InputValue()
	require.NoError(t, err)
	assert.Equal(t, "Ada Lovelace", got)

	require.NoError(t, worker.Fill(ctx, note.Ref, "Because I love Go."))
	got, err = worker.page.Locator("#cover-note").InputValue()
	require.NoError(t, err)
	assert.Equal(t, "Because I love Go.", got)

	require.NoError(t, worker.Fill(ctx, country.Ref, "United Kingdom"))
	got, err = worker.page.Locator("#country").InputValue()
	require.NoError(t, err)
	assert.NotEmpty(t, got)

	require.NoError(t, worker.Fill(ctx, agree.Ref, "yes"))
	checked, err := worker.page.Locator("input[name='agree']").IsChecked()
	require.NoError(t, err)
	assert.True(t, checked)

	require.NoError(t, worker.Fill(ctx, authorized.Ref, "No"))
	checked, err = worker.page.Locator("input[name='authorized'][value='no']").IsChecked()
	require.NoError(t, err)
	assert.True(t, checked)
}

func TestPlaywrightWorker_ExtractContent(t *testing.T) {
	server := newFixtureServer(t)
	worker := newTestWorker(t)
	ctx := context.Background()

	require.NoError(t, worker.Navigate(ctx, server.URL))

	content, err := worker.ExtractContent(ctx)

	require.NoError(t, err)
	assert.Contains(t, content, "Why do you want to work here?", "should return the real rendered page, not an empty shell")
}

// TestChromeSession_ConcurrentWorkersShareOneBrowser is the regression test
// for the defect that made "Prepare in browser" usable exactly once per
// daemon: a worker per run launched a second Chrome on a profile directory
// the first run's still-open window had locked, and every run after the
// first died at launch with "the profile is already in use by another
// instance of Chromium".
func TestChromeSession_ConcurrentWorkersShareOneBrowser(t *testing.T) {
	server := newFixtureServer(t)
	session := newTestSession(t)
	ctx := context.Background()

	first, err := session.NewWorker()
	if err != nil {
		t.Skipf("skipping: real Chrome/Playwright driver not available in this environment: %v", err)
	}
	require.NoError(t, first.Navigate(ctx, server.URL))

	// The first run's page is still open, exactly as a real one would be.
	second, err := session.NewWorker()
	require.NoError(t, err, "a second run must not be blocked by the first run's open window")
	require.NoError(t, second.Navigate(ctx, server.URL))

	fields, err := second.ExtractForm(ctx)
	require.NoError(t, err)
	assert.Len(t, fields, 5, "the second worker must drive a page of its own")

	// Closing one worker must not disturb the other's page.
	require.NoError(t, second.Close(ctx))
	_, err = first.ExtractForm(ctx)
	assert.NoError(t, err, "closing one run's tab must leave the other run's tab usable")
}

// TestChromeSession_RelaunchesAfterTheUserClosesChrome covers the other
// half: the window is the user's to close, and the next task should open a
// new one rather than fail on handles to a browser that is gone.
func TestChromeSession_RelaunchesAfterTheUserClosesChrome(t *testing.T) {
	server := newFixtureServer(t)
	session := newTestSession(t)
	ctx := context.Background()

	if _, err := session.NewWorker(); err != nil {
		t.Skipf("skipping: real Chrome/Playwright driver not available in this environment: %v", err)
	}
	session.Shutdown()

	worker, err := session.NewWorker()
	require.NoError(t, err, "a closed browser must be relaunched, not reported as a failure")
	require.NoError(t, worker.Navigate(ctx, server.URL))
}

// blockerFixture serves a page whose only interesting feature is `body`, so
// each DetectBlocker case can state exactly what it is testing.
func newBlockerFixture(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><html><body>" + body + "</body></html>"))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPlaywrightWorker_DetectBlocker(t *testing.T) {
	// A challenge frame big enough to cover a form, as posed reCAPTCHA is.
	const challengeFrame = `<iframe src="https://www.google.com/recaptcha/api2/bframe?x=1"
		style="width:400px;height:580px"></iframe>`

	tests := []struct {
		name        string
		body        string
		wantBlocked bool
	}{
		{
			// The regression: an invisible reCAPTCHA sits on real, entirely
			// fillable ATS forms (measured on Ashby). Flagging it failed
			// every application the worker could reach.
			name: "an invisible reCAPTCHA beside a usable form is not a blocker",
			body: `<iframe src="https://www.recaptcha.net/recaptcha/api2/anchor?ar=1"
				style="width:256px;height:60px;visibility:hidden"></iframe>
				<form><label for="n">Name</label><input id="n"></form>`,
			wantBlocked: false,
		},
		{
			// A checkbox is the human's to tick — they submit the form
			// themselves, so it stops nothing this worker does.
			name: "a visible reCAPTCHA checkbox is not a blocker",
			body: `<iframe src="https://www.google.com/recaptcha/api2/anchor?ar=1"
				style="width:304px;height:78px"></iframe>`,
			wantBlocked: false,
		},
		{
			name:        "a posed challenge covering the page is a blocker",
			body:        challengeFrame,
			wantBlocked: true,
		},
		{
			name:        "an interstitial's visible text is a blocker",
			body:        `<h1>Verify you are human</h1>`,
			wantBlocked: true,
		},
		{
			// The other false positive: page source matched everything,
			// including these words inside a script on a working page.
			name: "blocking words in scripts and hidden markup are not a blocker",
			body: `<script>const msg = "are you a robot";</script>
				<div hidden>unusual traffic</div>
				<form><label for="e">Email</label><input id="e"></form>`,
			wantBlocked: false,
		},
		{
			name:        "an ordinary page is not a blocker",
			body:        `<form><label for="n">Full name</label><input id="n"></form>`,
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newBlockerFixture(t, tt.body)
			worker := newTestWorker(t)
			ctx := context.Background()
			require.NoError(t, worker.Navigate(ctx, server.URL))

			reason, blocked := worker.DetectBlocker(ctx)

			assert.Equal(t, tt.wantBlocked, blocked, "reason: %q", reason)
			if tt.wantBlocked {
				assert.NotEmpty(t, reason, "a blocked page must say why")
			}
		})
	}
}

// TestPlaywrightWorker_FollowsApplyControlAcrossPages is the browser half of
// ADR-0013: the posting is rarely the form, so the worker has to be able to
// see a page's controls and click one. Which one is core's decision
// (services.PickApplyControl); this covers reporting and clicking.
func TestPlaywrightWorker_FollowsApplyControlAcrossPages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs/1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>
			<a href="/jobs/berlin">Jobs in Berlin</a>
			<a href="/jobs/1/apply">Apply Now</a>
			<a href="/hidden" style="display:none">Apply hidden</a>
			<form><input type="email" placeholder="Enter your email"></form>
		</body></html>`))
	})
	mux.HandleFunc("/jobs/1/apply", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body><h1>Application</h1>
			<label for="n">Full name</label><input id="n">
		</body></html>`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	worker := newTestWorker(t)
	ctx := context.Background()
	require.NoError(t, worker.Navigate(ctx, server.URL+"/jobs/1"))

	controls, err := worker.FindApplyControls(ctx)
	require.NoError(t, err)

	byText := map[string]ports.ApplyControl{}
	for _, c := range controls {
		byText[c.Text] = c
	}
	apply, ok := byText["Apply Now"]
	require.True(t, ok, "the apply link must be reported, got %+v", controls)
	assert.Equal(t, server.URL+"/jobs/1/apply", apply.URL, "a relative href must arrive resolved, so core can see a hand-off to another origin")
	assert.Contains(t, byText, "Jobs in Berlin", "filtering is core's job, so unrelated links are reported too")
	assert.NotContains(t, byText, "Apply hidden", "a control nobody can see is not a way in")

	require.NoError(t, worker.FollowControl(ctx, apply.Ref))
	assert.Equal(t, server.URL+"/jobs/1/apply", worker.CurrentURL(ctx))

	fields, err := worker.ExtractForm(ctx)
	require.NoError(t, err)
	require.Len(t, fields, 1)
	assert.Equal(t, "Full name", fields[0].Label, "the page it landed on is the one it now reads")
}

// TestPlaywrightWorker_FollowsApplyControlIntoANewTab is the regression test
// for a run that clicked "Apply" twice and then reported that it could not
// find the form. Both current boards send applicants off with
// target="_blank", so the click opens a tab and leaves the worker's own page
// sitting on the posting: it reads the listing again, decides it still is
// not a form, and clicks again until it runs out of hops.
func TestPlaywrightWorker_FollowsApplyControlIntoANewTab(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/posting", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>
			<a href="/form" target="_blank" rel="noopener">Apply Now</a>
			<form><input type="email" placeholder="Enter your email"></form>
		</body></html>`))
	})
	mux.HandleFunc("/form", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>
			<label for="n">Full name</label><input id="n">
			<label for="e">Email</label><input id="e">
			<label for="p">Phone</label><input id="p">
		</body></html>`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	worker := newTestWorker(t)
	ctx := context.Background()
	require.NoError(t, worker.Navigate(ctx, server.URL+"/posting"))

	controls, err := worker.FindApplyControls(ctx)
	require.NoError(t, err)
	var applyRef string
	for _, c := range controls {
		if c.Text == "Apply Now" {
			applyRef = c.Ref
		}
	}
	require.NotEmpty(t, applyRef)

	require.NoError(t, worker.FollowControl(ctx, applyRef))

	assert.Equal(t, server.URL+"/form", worker.CurrentURL(ctx), "the run must continue in the tab the click opened")

	fields, err := worker.ExtractForm(ctx)
	require.NoError(t, err)
	assert.Len(t, fields, 3, "and must read the form there, not the posting it came from")
}
