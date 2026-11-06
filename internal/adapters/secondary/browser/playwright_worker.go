// Package browser provides a ports.BrowserWorker implementation backed by
// a real, installed Chrome browser via github.com/mxschmitt/playwright-go,
// per docs/09_decisions/0005-go-playwright.md and
// docs/01_architecture/03_browser_worker.md.
package browser

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mxschmitt/playwright-go"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// captchaFrameSelector matches the iframes anti-bot vendors inject. It is a
// best-effort heuristic, not an anti-bot solver: "never bypass CAPTCHAs or
// anti-bot protections" means this only ever detects and stops, never
// attempts to defeat one.
const captchaFrameSelector = `iframe[src*="recaptcha" i], iframe[src*="hcaptcha" i], ` +
	`iframe[src*="turnstile" i], iframe[src*="arkoselabs" i]`

// challengeMinHeight separates a challenge being posed from a vendor script
// merely being present, measured on real pages in September 2026:
//
//	Ashby application form, entirely fillable:  anchor 256x60, visibility hidden
//	Google's reCAPTCHA demo, checkbox shown:    anchor 304x78, visible
//	Google's reCAPTCHA demo, challenge closed:  bframe 300x150, hidden
//
// An invisible frame blocks nothing, and a visible checkbox or badge sits
// beside content that can still be read. Only a challenge dialog, which
// covers the page and is several hundred pixels tall, stops the read.
//
// Flagging presence instead of a challenge is not the safe direction it
// looks like: the Ashby page above loads an invisible reCAPTCHA, which a
// URL-substring check would have refused to read.
const challengeMinHeight = 200

// blockerPhrases appear on interstitial pages that replace the content
// entirely. They are matched against rendered text, never page source: the
// same words sit in the scripts of pages that are working perfectly well.
//
// "performing security verification" is Cloudflare's challenge page, whose
// "Verify you are human" lives inside its widget's frame and so is not part
// of the page's own text (measured on himalayas.app, ADR-0033).
var blockerPhrases = []string{
	"verify you are human", "are you a robot", "unusual traffic", "performing security verification",
}

// challengePageScript reports whether the page is Cloudflare's challenge
// page, which defines this and nothing else does. Its checkbox is in a
// closed shadow root, so no selector can see the frame (ADR-0033 §1).
const challengePageScript = `() => typeof window._cf_chl_opt !== 'undefined'`

// PlaywrightWorker is a ports.BrowserWorker driving one page (one tab) of
// the shared JobPilot Chrome session.
type PlaywrightWorker struct {
	page  playwright.Page
	watch *pageWatch
}

// pageWatch is what the page's own events say about it, recorded as they
// arrive. Handlers run on Playwright's dispatcher, where a call back into
// the browser would deadlock, so they only ever store what the event
// already carries.
type pageWatch struct {
	// challenged is whether the page's current document arrived marked
	// "cf-mitigated: challenge", Cloudflare's own signal that the response
	// is a challenge rather than the page asked for.
	challenged atomic.Bool
}

func newPlaywrightWorker(page playwright.Page) *PlaywrightWorker {
	return &PlaywrightWorker{page: page, watch: watchPage(page)}
}

func watchPage(page playwright.Page) *pageWatch {
	watch := &pageWatch{}
	page.OnResponse(func(response playwright.Response) {
		request := response.Request()
		if !request.IsNavigationRequest() {
			return
		}
		if frame := request.Frame(); frame == nil || frame.ParentFrame() != nil {
			return
		}
		watch.challenged.Store(strings.EqualFold(response.Headers()["cf-mitigated"], "challenge"))
	})
	return watch
}

// ChromeSession owns the single Chrome process behind every browser task.
//
// One process is not an optimization, it is the only thing Chrome permits:
// a user-data-dir may be open in exactly one instance, so two imports at
// once each get a page of the same browser rather than a browser each.
//
// Chrome is launched lazily on first use and relaunched if the user closes
// it, so the daemon holds no browser open for a user who never asks for one.
type ChromeSession struct {
	profileDir string
	headless   bool

	mu sync.Mutex
	pw *playwright.Playwright
	// context is nil whenever no Chrome is running.
	context playwright.BrowserContext
	// initialPageFree tracks the blank page Chrome opens with, so the first
	// worker adopts it instead of leaving a stray about:blank tab behind.
	initialPageFree bool
}

// NewChromeSession describes a Chrome session over a persistent,
// JobPilot-owned profile at profileDir — never the user's own daily-driver
// profile, so this never conflicts with their regular browser and never
// needs their real passwords. headless should only be true in automated
// tests: real use shows the window, so a page that will not load, or asks
// whether the user is a person, is not hidden from them.
//
// No browser starts until the first NewWorker call.
func NewChromeSession(profileDir string, headless bool) *ChromeSession {
	return &ChromeSession{profileDir: profileDir, headless: headless}
}

// NewWorker returns a worker bound to a new page in the shared browser,
// starting Chrome if it is not already running.
func (s *ChromeSession) NewWorker() (ports.BrowserWorker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.context != nil {
		page, err := s.pageLocked()
		if err == nil {
			return newPlaywrightWorker(page), nil
		}
		// The user closed the window, so these handles are dead. Drop them
		// and start over rather than reporting their failure as this run's.
		s.discardLocked()
	}

	if err := s.launchLocked(); err != nil {
		return nil, err
	}
	page, err := s.pageLocked()
	if err != nil {
		s.discardLocked()
		return nil, err
	}
	return newPlaywrightWorker(page), nil
}

// Shutdown closes the browser, if one is running, for daemon shutdown.
func (s *ChromeSession) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discardLocked()
}

// ProfileEraser removes this session's Chrome profile, for "delete all
// data" (ADR-0021 §7): the profile holds whatever job boards the user
// signed into, which is theirs, not the browser's.
func (s *ChromeSession) ProfileEraser() ports.LocalDataEraser {
	return profileEraser{session: s}
}

type profileEraser struct{ session *ChromeSession }

func (p profileEraser) Describe() string {
	return "the browser profile and its job board sign-ins"
}

// Erase closes Chrome, which holds the profile open, and removes it. The
// session's lock is held throughout, so no task can relaunch Chrome on the
// directory while it is being removed. A missing profile is already erased.
func (p profileEraser) Erase(context.Context) error {
	s := p.session
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discardLocked()
	if err := os.RemoveAll(s.profileDir); err != nil {
		return fmt.Errorf("failed to remove the browser profile: %w", err)
	}
	return nil
}

func (s *ChromeSession) launchLocked() error {
	// SkipInstallBrowsers: the launch below uses Channel: "chrome" (the
	// user's real installed Chrome per architecture principle #1), so
	// Playwright's own bundled Chromium/Firefox/WebKit binaries are never
	// needed — only its driver, keeping the one-time download small.
	if err := playwright.Install(&playwright.RunOptions{SkipInstallBrowsers: true}); err != nil {
		return fmt.Errorf("failed to install playwright driver: %w", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("failed to start playwright: %w", err)
	}

	// ChromiumSandbox: Playwright passes --no-sandbox unless asked not to,
	// and this browser reads untrusted pages (ADR-0033 §6).
	browserContext, err := pw.Chromium.LaunchPersistentContext(s.profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Channel:         new("chrome"),
		Headless:        new(s.headless),
		ChromiumSandbox: new(true),
	})
	if err != nil {
		_ = pw.Stop()
		return launchError(err)
	}

	s.pw = pw
	s.context = browserContext
	s.initialPageFree = len(browserContext.Pages()) > 0
	return nil
}

// pageLocked hands out the browser's initial blank page once, then opens a
// new tab per worker.
func (s *ChromeSession) pageLocked() (playwright.Page, error) {
	if s.initialPageFree {
		if pages := s.context.Pages(); len(pages) > 0 {
			s.initialPageFree = false
			return pages[0], nil
		}
		s.initialPageFree = false
	}

	page, err := s.context.NewPage()
	if err != nil {
		return nil, fmt.Errorf("failed to open a browser tab: %w", err)
	}
	return page, nil
}

func (s *ChromeSession) discardLocked() {
	if s.context != nil {
		_ = s.context.Close()
		s.context = nil
	}
	if s.pw != nil {
		_ = s.pw.Stop()
		s.pw = nil
	}
	s.initialPageFree = false
}

// launchError turns Playwright's launch failures into something the user
// can act on. The profile-in-use case is the one they will actually hit —
// a JobPilot window left open by an earlier daemon, which this process
// cannot adopt — and Playwright reports it as a wall of Chrome flags.
func launchError(err error) error {
	if strings.Contains(err.Error(), "profile is already in use") {
		return fmt.Errorf("a JobPilot browser window from an earlier session is still open — close it and try again")
	}
	return fmt.Errorf("failed to launch Chrome (is Google Chrome installed?): %w", err)
}

// Navigate opens url in the worker's page.
func (w *PlaywrightWorker) Navigate(_ context.Context, url string) error {
	if _, err := w.page.Goto(url); err != nil {
		return fmt.Errorf("failed to navigate to %s: %w", url, err)
	}
	return nil
}

// DetectBlocker reports whether the page is posing a challenge that stops
// it being read. See challengeMinHeight for why presence of a CAPTCHA is
// not itself the test, and ADR-0033 §1 for a page that is the challenge.
func (w *PlaywrightWorker) DetectBlocker(_ context.Context) (string, bool) {
	if w.watch.challenged.Load() {
		return w.checkingReason(), true
	}
	if isChallenge, err := w.page.Evaluate(challengePageScript); err == nil && isChallenge == true {
		return w.checkingReason(), true
	}

	frames := w.page.Locator(captchaFrameSelector)
	count, err := frames.Count()
	if err != nil {
		count = 0
	}
	for i := range count {
		frame := frames.Nth(i)
		visible, err := frame.IsVisible()
		if err != nil || !visible {
			continue
		}
		box, err := frame.BoundingBox()
		if err != nil || box == nil || box.Height < challengeMinHeight {
			continue
		}
		return "a CAPTCHA challenge is covering the page", true
	}

	if text, err := w.page.Locator("body").InnerText(); err == nil {
		lower := strings.ToLower(text)
		for _, phrase := range blockerPhrases {
			if strings.Contains(lower, phrase) {
				return w.checkingReason(), true
			}
		}
	}

	return "", false
}

// checkingReason names the site doing the checking.
func (w *PlaywrightWorker) checkingReason() string {
	site := "the site"
	if parsed, err := url.Parse(w.page.URL()); err == nil && parsed.Hostname() != "" {
		site = parsed.Hostname()
	}
	return site + " is checking that you're a person"
}

// ExtractContent returns the current page's HTML.
func (w *PlaywrightWorker) ExtractContent(_ context.Context) (string, error) {
	content, err := w.page.Content()
	if err != nil {
		return "", fmt.Errorf("failed to read page content: %w", err)
	}
	return content, nil
}

// Close closes this worker's tab, leaving the rest of the session
// untouched.
func (w *PlaywrightWorker) Close(_ context.Context) error {
	if err := w.page.Close(); err != nil {
		return fmt.Errorf("failed to close browser tab: %w", err)
	}
	return nil
}
