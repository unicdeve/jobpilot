//go:build integration

package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixturePostingHTML is a small job-posting-shaped page, which is what the
// worker reads when a job is imported from a link.
const fixturePostingHTML = `<!doctype html>
<html>
<body>
<h1>Senior Backend Engineer</h1>
<p>Why you will love working here: a small team building in Go.</p>
<a href="/apply">Apply for this job</a>
</body>
</html>`

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fixturePostingHTML))
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

func TestPlaywrightWorker_ExtractContent(t *testing.T) {
	server := newFixtureServer(t)
	worker := newTestWorker(t)
	ctx := context.Background()

	require.NoError(t, worker.Navigate(ctx, server.URL))

	content, err := worker.ExtractContent(ctx)

	require.NoError(t, err)
	assert.Contains(t, content, "a small team building in Go", "should return the real rendered page, not an empty shell")
}

// TestChromeSession_ConcurrentWorkersShareOneBrowser covers two imports at
// once: Chrome allows one instance per profile directory, so a worker per
// import launching its own Chrome would fail every import after the first
// with "the profile is already in use by another instance of Chromium".
func TestChromeSession_ConcurrentWorkersShareOneBrowser(t *testing.T) {
	server := newFixtureServer(t)
	session := newTestSession(t)
	ctx := context.Background()

	first, err := session.NewWorker()
	if err != nil {
		t.Skipf("skipping: real Chrome/Playwright driver not available in this environment: %v", err)
	}
	require.NoError(t, first.Navigate(ctx, server.URL))

	second, err := session.NewWorker()
	require.NoError(t, err, "a second worker must not be blocked by the first one's open page")
	require.NoError(t, second.Navigate(ctx, server.URL))
	_, err = second.ExtractContent(ctx)
	require.NoError(t, err)

	// Closing one worker must not disturb the other's page.
	require.NoError(t, second.Close(ctx))
	content, err := first.ExtractContent(ctx)
	require.NoError(t, err, "closing one worker's tab must leave the other's usable")
	assert.Contains(t, content, "Senior Backend Engineer")
}

// TestChromeSession_RelaunchesAfterTheUserClosesChrome: the window is the
// user's to close, and the next import should open a new one rather than
// fail on handles to a browser that is gone.
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

// cloudflareChallengeBody is Cloudflare's challenge page as it stood in front
// of a Himalayas posting on 2026-10-24, reduced to what DetectBlocker could
// read: the visible text, and the script variable only that page defines.
// The checkbox itself sits in a closed shadow root no selector can reach,
// so it is left out — as it is, in effect, for the worker.
const cloudflareChallengeBody = `<div class="main-wrapper">
	<h1>himalayas.app</h1>
	<h2>Performing security verification</h2>
	<p>This website uses a security service to protect against malicious bots.
	This page is displayed while the website verifies you are not a bot.</p>
	<p>Ray ID: a4fab21bba3cb97b</p><p>Performance and Security by Cloudflare</p>
</div>`

// TestPlaywrightWorker_DetectsAPageThatIsTheCheck covers ADR-0033 §1: the
// page Cloudflare serves instead of the one asked for. Each signal is
// tested alone, since each is the one left when the others change.
func TestPlaywrightWorker_DetectsAPageThatIsTheCheck(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		body        string
		wantBlocked bool
	}{
		{
			name:        "the page as measured",
			header:      "challenge",
			body:        cloudflareChallengeBody + `<script>window._cf_chl_opt = {};</script>`,
			wantBlocked: true,
		},
		{
			name:        "Cloudflare's header alone",
			header:      "challenge",
			body:        `<p>Just a moment...</p>`,
			wantBlocked: true,
		},
		{
			name:        "the challenge page's script alone",
			body:        `<script>window._cf_chl_opt = {cType: 'managed'};</script>`,
			wantBlocked: true,
		},
		{
			name:        "its visible text alone",
			body:        cloudflareChallengeBody,
			wantBlocked: true,
		},
		{
			// Turnstile on an ordinary form is a checkbox the user ticks
			// before submitting, like reCAPTCHA's (ADR-0013 §4).
			name: "a Turnstile widget beside a form is not a blocker",
			body: `<iframe src="https://challenges.cloudflare.com/cdn-cgi/challenge-platform/turnstile/if/ov2"
				style="width:300px;height:65px"></iframe>
				<form><label for="n">Full name</label><input id="n"></form>`,
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				status := http.StatusOK
				if tt.header != "" {
					w.Header().Set("cf-mitigated", tt.header)
					status = http.StatusForbidden
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("<!doctype html><html><head><title>Just a moment...</title></head><body>" +
					tt.body + "</body></html>"))
			}))
			t.Cleanup(server.Close)
			worker := newTestWorker(t)
			ctx := context.Background()
			require.NoError(t, worker.Navigate(ctx, server.URL))

			reason, blocked := worker.DetectBlocker(ctx)

			assert.Equal(t, tt.wantBlocked, blocked, "reason: %q", reason)
			if tt.wantBlocked {
				assert.Equal(t, "127.0.0.1 is checking that you're a person", reason,
					"the reason names the site, so the user knows which window to look in")
			}
		})
	}
}

// TestPlaywrightWorker_ACheckThatPassesClears is the other half: a check
// served again is still seen, and once the page is the one asked for nothing
// is blocking: the header belongs to its document, not the tab. Every reload
// is counted.
func TestPlaywrightWorker_ACheckThatPassesClears(t *testing.T) {
	var passed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if !passed.Load() {
			w.Header().Set("cf-mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<!doctype html><html><body>" + cloudflareChallengeBody + "</body></html>"))
			return
		}
		_, _ = w.Write([]byte(`<!doctype html><html><body><h1>Frontend engineer</h1>
			<a href="/apply">Apply</a></body></html>`))
	}))
	t.Cleanup(server.Close)
	worker := newTestWorker(t)
	ctx := context.Background()
	require.NoError(t, worker.Navigate(ctx, server.URL))

	_, blocked := worker.DetectBlocker(ctx)
	require.True(t, blocked)

	// Served again: the page reloads, and the check is back.
	_, err := worker.page.Reload()
	require.NoError(t, err)
	_, blocked = worker.DetectBlocker(ctx)
	assert.True(t, blocked, "a check served again is still a check")

	passed.Store(true)
	_, err = worker.page.Reload()
	require.NoError(t, err)

	reason, blocked := worker.DetectBlocker(ctx)
	assert.False(t, blocked, "the page asked for is not a check: %q", reason)
}
