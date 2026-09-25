// Package browser provides a ports.BrowserWorker implementation backed by
// a real, installed Chrome browser via github.com/mxschmitt/playwright-go,
// per docs/09_decisions/0005-go-playwright.md and
// docs/01_architecture/03_browser_worker.md.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

const formFieldSelector = "input, select, textarea"

// skipInputTypes are <input> types that are never meaningful application
// questions: hidden/submit/button/reset are form plumbing, and file inputs
// (e.g. resume upload) can't be filled from a text answer, so they're left
// for the human in the still-open browser window rather than treated as a
// blocker.
var skipInputTypes = map[string]bool{
	"hidden": true, "submit": true, "button": true, "reset": true, "image": true, "file": true,
}

// captchaFrameSelector matches the iframes anti-bot vendors inject. It is a
// best-effort heuristic, not an anti-bot solver — "never bypass CAPTCHAs or
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
// beside a form the human can still complete — they submit it themselves,
// so ticking a box is their job, not a reason to abandon the run. Only a
// challenge dialog, which covers the form and is several hundred pixels
// tall, actually stops the work.
//
// Flagging presence instead of a challenge is not the safe direction it
// looks like: the Ashby page above loads an invisible reCAPTCHA, so the old
// URL-substring check would have failed every application it could reach.
const challengeMinHeight = 200

// blockerPhrases appear on interstitial pages that replace the content
// entirely. They are matched against rendered text, never page source: the
// same words sit in the scripts of pages that are working perfectly well.
var blockerPhrases = []string{"verify you are human", "are you a robot", "unusual traffic"}

// PlaywrightWorker is a ports.BrowserWorker driving one page (one tab) of
// the shared JobPilot Chrome session.
type PlaywrightWorker struct {
	page playwright.Page
}

// ChromeSession owns the single Chrome process behind every browser task.
//
// One process is not an optimization, it is the only thing Chrome permits:
// a user-data-dir may be open in exactly one instance. Runs deliberately
// leave their window open for the human to submit in, so launching a second
// Chrome on the same profile — which is what a worker-per-run did — could
// only ever fail with "the profile is already in use by another instance of
// Chromium" for every run after the first. Sessions hand out a fresh page
// instead, which keeps what the per-run factory was actually for: each
// prepared application gets its own tab, left open, never overwritten by
// the next.
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
// needs their real passwords; they log into any job board once inside this
// profile. headless should only be true in automated tests: real use always
// shows a visible window so the human can review and submit themselves, per
// docs/01_architecture/03_browser_worker.md.
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
			return &PlaywrightWorker{page: page}, nil
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
	return &PlaywrightWorker{page: page}, nil
}

// Shutdown closes the browser, if one is running. It is for daemon
// shutdown: ordinary task completion leaves the window open on purpose.
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

	browserContext, err := pw.Chromium.LaunchPersistentContext(s.profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Channel:  new("chrome"),
		Headless: new(s.headless),
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

// DetectBlocker reports whether the page is posing a challenge the run
// cannot get past. See challengeMinHeight for why presence of a CAPTCHA is
// not itself the test.
func (w *PlaywrightWorker) DetectBlocker(_ context.Context) (string, bool) {
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
				return fmt.Sprintf("the page contains blocking text: %q", phrase), true
			}
		}
	}

	return "", false
}

// applyControlSelector matches anything a page might use as a way in. It is
// deliberately broad: narrowing is core's job (services.PickApplyControl),
// so the rule that decides can be tested without a browser.
const applyControlSelector = `a[href], button, [role="button"]`

// maxApplyControls bounds the scan. A job posting's navigation, footer and
// related-jobs list can run to hundreds of links, and the way in is never
// the four-hundredth of them.
const maxApplyControls = 300

// describeControlScript reads a control's visible text and resolved
// destination. href is read from the property rather than the attribute so
// a relative link arrives absolute, which is what core needs to recognise
// a hand-off to another origin.
const describeControlScript = `(el) => ({
	text: (el.innerText || el.getAttribute('aria-label') || el.getAttribute('title') || '').trim(),
	url: el.tagName.toLowerCase() === 'a' ? (el.href || '') : '',
})`

// CurrentURL is where the page actually is, after any redirect.
func (w *PlaywrightWorker) CurrentURL(_ context.Context) string {
	return w.page.URL()
}

// FindApplyControls lists the page's visible links and buttons.
func (w *PlaywrightWorker) FindApplyControls(_ context.Context) ([]ports.ApplyControl, error) {
	locator := w.page.Locator(applyControlSelector)
	count, err := locator.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count page controls: %w", err)
	}
	count = min(count, maxApplyControls)

	controls := make([]ports.ApplyControl, 0, count)
	for i := range count {
		item := locator.Nth(i)
		if visible, err := item.IsVisible(); err != nil || !visible {
			continue
		}

		result, err := item.Evaluate(describeControlScript, nil)
		if err != nil {
			continue
		}
		described, ok := result.(map[string]any)
		if !ok {
			continue
		}
		text, _ := described["text"].(string)
		target, _ := described["url"].(string)
		if text == "" && target == "" {
			continue
		}
		controls = append(controls, ports.ApplyControl{
			Ref:  strconv.Itoa(i),
			Text: text,
			URL:  target,
		})
	}
	return controls, nil
}

// FollowControl clicks the control and waits for the page to settle,
// whether it navigates by href, by script, or not at all.
func (w *PlaywrightWorker) FollowControl(_ context.Context, ref string) error {
	index, err := strconv.Atoi(ref)
	if err != nil {
		return fmt.Errorf("invalid control reference %q", ref)
	}

	locator := w.page.Locator(applyControlSelector)
	count, err := locator.Count()
	if err != nil {
		return fmt.Errorf("failed to locate page controls: %w", err)
	}
	if index < 0 || index >= count {
		return fmt.Errorf("control reference %d is out of range", index)
	}

	browserContext := w.page.Context()
	before := len(browserContext.Pages())

	if err := locator.Nth(index).Click(); err != nil {
		return fmt.Errorf("failed to click the apply control: %w", err)
	}

	// Job boards almost always send applicants off with target="_blank" —
	// both connectors' apply links do — so the page the run continues on is
	// usually not the page it clicked. Without adopting the new tab the
	// worker reads the posting again, decides it still is not a form, and
	// clicks apply a second time.
	if adopted := waitForNewPage(browserContext, before); adopted != nil {
		w.page = adopted
	}

	// A control may navigate, may open a tab, may swap the form in without
	// navigating at all. Waiting for the network to settle covers them; a
	// page that never settles is not a failure to follow, so a timeout here
	// is not fatal — the caller judges the page it ends up on.
	_ = w.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(newPageTimeout),
	})
	return nil
}

// newPageTimeout bounds both the wait for a new tab and the wait for the
// resulting page to settle, in milliseconds.
const newPageTimeout = 10_000

// waitForNewPage returns the tab a click opened, or nil if it opened none.
// Playwright's popup helpers report a missing popup as an error, and a
// control that navigates in place is an ordinary outcome here rather than a
// failure, so this polls instead.
func waitForNewPage(browserContext playwright.BrowserContext, before int) playwright.Page {
	deadline := time.Now().Add(newPageTimeout * time.Millisecond)
	for time.Now().Before(deadline) {
		if pages := browserContext.Pages(); len(pages) > before {
			return pages[len(pages)-1]
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// minIdentifierLabelLength is where a label stops being a word and starts
// being an id. Short all-hex strings are ordinary English ("fee", "added",
// "cab"); long ones are machine names that leaked through the label chain.
const minIdentifierLabelLength = 12

// isReadableLabel reports whether a label is something a person could
// answer, as opposed to a machine name that reached the end of the
// accessible-name chain.
//
// Ashby's radio groups have no <fieldset><legend> and UUID name attributes,
// so the fallback produced questions labelled
// "ce2a003d 1cd3 4cab 82ae 298b5f8df5f3 ..." — unanswerable by the user and
// useless to the resolver. A field dropped here is not lost: the browser
// window stays open on the form, which is where the human fills in anything
// the worker left alone, exactly as it does for a question nothing could
// resolve.
func isReadableLabel(label string) bool {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return false
	}
	if machineIDPattern.MatchString(trimmed) {
		return false
	}
	for _, token := range strings.Fields(trimmed) {
		if containsNonHexLetter(token) {
			return true
		}
	}
	// Every token could be a fragment of hex. Length decides which it is.
	return len(trimmed) < minIdentifierLabelLength
}

// requiredMarkerSuffix matches the asterisk a form puts at the end of a
// label to mean "required". It is already carried on FormField.Required, so
// leaving it in the text only shows the user a question with punctuation
// they cannot answer — "First Name*", "Phone ✱".
var requiredMarkerSuffix = regexp.MustCompile(`[\s*✱＊†]+$`)

// cleanLabel is the question as a person would read it.
func cleanLabel(label string) string {
	return requiredMarkerSuffix.ReplaceAllString(strings.TrimSpace(label), "")
}

// machineIDPattern matches a UUID however the surrounding markup spells it:
// hyphenated, spaced, or run together. A question a person can answer never
// carries one, so a label holding one is a name attribute that reached the
// end of the accessible-name chain — and the real words around it are the
// markup's, not the employer's.
//
// Measured on the two forms this matters on:
//
//	Ashby   "ce2a003d 1cd3 4cab 82ae 298b5f8df5f3"   (a humanized name)
//	Lever   "cards[e27c9c9a-4ad6-4889-b355-8862299c8274][field0]"
//
// The second is why a label being entirely an identifier is not the test:
// Lever's reads as words to anything looking token by token.
var machineIDPattern = regexp.MustCompile(
	`(?i)[0-9a-f]{8}[- ]?[0-9a-f]{4}[- ]?[0-9a-f]{4}[- ]?[0-9a-f]{4}[- ]?[0-9a-f]{12}`)

// containsNonHexLetter reports whether a token holds a letter that could not
// appear in a hex id, which is the cheapest evidence that it is a word.
func containsNonHexLetter(token string) bool {
	for _, r := range strings.ToLower(token) {
		if r >= 'g' && r <= 'z' {
			return true
		}
	}
	return false
}

// formLocator matches every candidate field on the current page.
func (w *PlaywrightWorker) formLocator() playwright.Locator {
	return w.page.Locator(formFieldSelector)
}

// extractedField mirrors extractFieldScript's return shape.
type extractedField struct {
	Tag        string   `json:"tag"`
	Type       string   `json:"type"`
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Required   bool     `json:"required"`
	Options    []string `json:"options"`
	GroupLabel string   `json:"groupLabel"`
}

// extractFieldScript computes a field's accessible label using the
// standard association chain browsers use for accessible names
// (<label for>, a wrapping <label>, aria-label, placeholder, then the name
// attribute as a last resort) rather than reinventing it, plus its
// enclosing <fieldset><legend> text for grouping radio buttons.
const extractFieldScript = `(el) => {
	// What a person can see, not what the markup holds: a Lever label wraps
	// its field along with the hidden "No location found" and "Loading"
	// text of an autocomplete that is not open, and textContent hands all
	// of it back as part of the question.
	const shown = (node) => (node.innerText || node.textContent || '').trim();
	const tag = el.tagName.toLowerCase();
	const type = (el.getAttribute('type') || '').toLowerCase();
	const name = el.getAttribute('name') || '';
	let label = '';
	if (el.id) {
		const esc = (window.CSS && CSS.escape) ? CSS.escape(el.id) : el.id;
		const l = document.querySelector('label[for="' + esc + '"]');
		if (l) label = shown(l);
	}
	if (!label) {
		const wrapping = el.closest('label');
		if (wrapping) label = shown(wrapping);
	}
	if (!label) label = el.getAttribute('aria-label') || '';
	if (!label) label = el.getAttribute('placeholder') || '';
	if (!label) label = name;
	const required = el.required === true || el.getAttribute('aria-required') === 'true';
	let options = [];
	if (tag === 'select') {
		options = Array.from(el.options).map((o) => o.textContent.trim());
	}
	let groupLabel = '';
	const fieldset = el.closest('fieldset');
	if (fieldset) {
		const legend = fieldset.querySelector('legend');
		if (legend) groupLabel = shown(legend);
	}
	return { tag, type, name, label, required, options, groupLabel };
}`

func (w *PlaywrightWorker) describeField(loc playwright.Locator) (extractedField, error) {
	result, err := loc.Evaluate(extractFieldScript, nil)
	if err != nil {
		return extractedField{}, fmt.Errorf("failed to inspect field: %w", err)
	}

	b, err := json.Marshal(result)
	if err != nil {
		return extractedField{}, fmt.Errorf("failed to encode field description: %w", err)
	}

	var ef extractedField
	if err := json.Unmarshal(b, &ef); err != nil {
		return extractedField{}, fmt.Errorf("failed to decode field description: %w", err)
	}
	return ef, nil
}

// ExtractForm walks every input/select/textarea on the current page,
// skipping hidden/plumbing/file fields, and groups radio buttons sharing a
// name into a single field so a yes/no-style question (very common on
// application forms — work authorization, sponsorship, self-identification)
// becomes one ApplicationQuestion instead of one per radio option.
func (w *PlaywrightWorker) ExtractForm(_ context.Context) ([]ports.FormField, error) {
	count, err := w.formLocator().Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count form fields: %w", err)
	}

	type indexedField struct {
		index int
		extractedField
	}

	raw := make([]indexedField, 0, count)
	for i := range count {
		item := w.formLocator().Nth(i)

		visible, err := item.IsVisible()
		if err != nil || !visible {
			continue
		}
		ef, err := w.describeField(item)
		if err != nil {
			continue
		}
		if ef.Tag == "input" && skipInputTypes[ef.Type] {
			continue
		}
		raw = append(raw, indexedField{index: i, extractedField: ef})
	}

	fields := make([]ports.FormField, 0, len(raw))
	seenRadioGroups := map[string]bool{}
	for _, f := range raw {
		if f.Tag == "input" && f.Type == "radio" {
			if f.Name == "" || seenRadioGroups[f.Name] {
				continue
			}
			seenRadioGroups[f.Name] = true

			var indices []int
			var options []string
			for _, g := range raw {
				if g.Tag == "input" && g.Type == "radio" && g.Name == f.Name {
					indices = append(indices, g.index)
					options = append(options, cleanLabel(g.Label))
				}
			}

			label := cleanLabel(f.GroupLabel)
			if label == "" {
				label = humanizeName(f.Name)
			}
			if !isReadableLabel(label) {
				continue
			}
			refBytes, _ := json.Marshal(indices)
			fields = append(fields, ports.FormField{
				Ref:      string(refBytes),
				Label:    label,
				Kind:     ports.FormFieldKindRadio,
				Required: f.Required,
				Options:  options,
			})
			continue
		}

		label := cleanLabel(f.Label)
		if !isReadableLabel(label) {
			continue
		}
		fields = append(fields, ports.FormField{
			Ref:      strconv.Itoa(f.index),
			Label:    label,
			Kind:     kindOf(f.extractedField),
			Required: f.Required,
			Options:  f.Options,
		})
	}

	return fields, nil
}

func kindOf(f extractedField) ports.FormFieldKind {
	switch {
	case f.Tag == "select":
		return ports.FormFieldKindSelect
	case f.Tag == "textarea":
		return ports.FormFieldKindTextarea
	case f.Type == "checkbox":
		return ports.FormFieldKindCheckbox
	default:
		return ports.FormFieldKindText
	}
}

func humanizeName(name string) string {
	return strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(name))
}

// Fill fills the field referenced by fieldRef with value. fieldRef is
// either a plain field's index (as produced by ExtractForm) or a JSON array
// of indices for a grouped radio field.
func (w *PlaywrightWorker) Fill(_ context.Context, fieldRef, value string) error {
	var indices []int
	if err := json.Unmarshal([]byte(fieldRef), &indices); err == nil && len(indices) > 0 {
		return w.fillRadioGroup(indices, value)
	}

	idx, err := strconv.Atoi(fieldRef)
	if err != nil {
		return fmt.Errorf("invalid field reference %q", fieldRef)
	}

	count, err := w.formLocator().Count()
	if err != nil {
		return fmt.Errorf("failed to locate form fields: %w", err)
	}
	if idx < 0 || idx >= count {
		return fmt.Errorf("field reference %d is out of range", idx)
	}

	return w.fillLocator(w.formLocator().Nth(idx), value)
}

func (w *PlaywrightWorker) fillLocator(loc playwright.Locator, value string) error {
	ef, err := w.describeField(loc)
	if err != nil {
		return err
	}

	switch {
	case ef.Tag == "select":
		labels := []string{value}
		if _, err := loc.SelectOption(playwright.SelectOptionValues{Labels: &labels}); err != nil {
			return fmt.Errorf("failed to select option %q: %w", value, err)
		}
		return nil
	case ef.Type == "checkbox":
		if truthy(value) {
			if err := loc.Check(); err != nil {
				return fmt.Errorf("failed to check field: %w", err)
			}
			return nil
		}
		if err := loc.Uncheck(); err != nil {
			return fmt.Errorf("failed to uncheck field: %w", err)
		}
		return nil
	default:
		if err := loc.Fill(value); err != nil {
			return fmt.Errorf("failed to fill field: %w", err)
		}
		return nil
	}
}

func (w *PlaywrightWorker) fillRadioGroup(indices []int, value string) error {
	count, err := w.formLocator().Count()
	if err != nil {
		return fmt.Errorf("failed to locate form fields: %w", err)
	}

	for _, idx := range indices {
		if idx < 0 || idx >= count {
			continue
		}
		item := w.formLocator().Nth(idx)
		ef, err := w.describeField(item)
		if err != nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(ef.Label), strings.TrimSpace(value)) {
			if err := item.Check(); err != nil {
				return fmt.Errorf("failed to select radio option %q: %w", value, err)
			}
			return nil
		}
	}

	return fmt.Errorf("no radio option matched %q", value)
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "y", "1":
		return true
	default:
		return false
	}
}

// ExtractContent returns the current page's HTML, for a caller reading the
// page itself (e.g. importing a job posting) rather than filling a form.
func (w *PlaywrightWorker) ExtractContent(_ context.Context) (string, error) {
	content, err := w.page.Content()
	if err != nil {
		return "", fmt.Errorf("failed to read page content: %w", err)
	}
	return content, nil
}

// Close closes this worker's tab, leaving the rest of the session — and
// any other run's window — untouched. Callers must only invoke it when
// nothing useful has been shown to the user yet; every other path leaves
// the tab open deliberately.
func (w *PlaywrightWorker) Close(_ context.Context) error {
	if err := w.page.Close(); err != nil {
		return fmt.Errorf("failed to close browser tab: %w", err)
	}
	return nil
}
