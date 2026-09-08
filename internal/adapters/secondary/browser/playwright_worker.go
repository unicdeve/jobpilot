// Package browser provides a ports.BrowserWorker implementation backed by
// a real, installed Chrome browser via github.com/mxschmitt/playwright-go,
// per docs/09_decisions/0005-go-playwright.md and
// docs/01_architecture/03_browser_worker.md.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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

// captchaHostMarkers and blockerPhrases are a best-effort heuristic, not an
// anti-bot solver — "never bypass CAPTCHAs or anti-bot protections" means
// this only ever detects and stops, never attempts to defeat one. A false
// negative just means extraction/fill likely fails and stops there anyway;
// a false positive is the safe failure direction.
var captchaHostMarkers = []string{"recaptcha", "hcaptcha", "turnstile", "arkoselabs"}
var blockerPhrases = []string{"verify you are human", "are you a robot", "unusual traffic"}

// PlaywrightWorker is a ports.BrowserWorker backed by a real Chrome
// instance.
type PlaywrightWorker struct {
	pw      *playwright.Playwright
	context playwright.BrowserContext
	page    playwright.Page
}

// NewPlaywrightWorker launches a persistent, JobPilot-owned Chrome profile
// at profileDir — never the user's own daily-driver profile, so this never
// conflicts with their regular browser and never needs their real
// passwords; they log into any job board once inside this profile. headless
// should only be true in automated tests: real use always shows a visible
// window so the human can review and submit themselves, per
// docs/01_architecture/03_browser_worker.md.
func NewPlaywrightWorker(profileDir string, headless bool) (*PlaywrightWorker, error) {
	// SkipInstallBrowsers: every launch below uses Channel: "chrome" (the
	// user's real installed Chrome per architecture principle #1), so
	// Playwright's own bundled Chromium/Firefox/WebKit binaries are never
	// needed — only its driver, keeping the one-time download small.
	if err := playwright.Install(&playwright.RunOptions{SkipInstallBrowsers: true}); err != nil {
		return nil, fmt.Errorf("failed to install playwright driver: %w", err)
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to start playwright: %w", err)
	}

	browserContext, err := pw.Chromium.LaunchPersistentContext(profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Channel:  new("chrome"),
		Headless: new(headless),
	})
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("failed to launch Chrome (is Google Chrome installed?): %w", err)
	}

	page, err := firstOrNewPage(browserContext)
	if err != nil {
		_ = browserContext.Close()
		_ = pw.Stop()
		return nil, err
	}

	return &PlaywrightWorker{pw: pw, context: browserContext, page: page}, nil
}

func firstOrNewPage(ctx playwright.BrowserContext) (playwright.Page, error) {
	if pages := ctx.Pages(); len(pages) > 0 {
		return pages[0], nil
	}
	page, err := ctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("failed to open page: %w", err)
	}
	return page, nil
}

// Navigate opens url in the worker's page.
func (w *PlaywrightWorker) Navigate(_ context.Context, url string) error {
	if _, err := w.page.Goto(url); err != nil {
		return fmt.Errorf("failed to navigate to %s: %w", url, err)
	}
	return nil
}

// DetectBlocker checks the current page for known CAPTCHA/anti-bot markers.
func (w *PlaywrightWorker) DetectBlocker(_ context.Context) (string, bool) {
	for _, frame := range w.page.Frames() {
		url := strings.ToLower(frame.URL())
		for _, marker := range captchaHostMarkers {
			if strings.Contains(url, marker) {
				return fmt.Sprintf("a %s challenge is present on the page", marker), true
			}
		}
	}

	if content, err := w.page.Content(); err == nil {
		lower := strings.ToLower(content)
		for _, phrase := range blockerPhrases {
			if strings.Contains(lower, phrase) {
				return fmt.Sprintf("the page contains blocking text: %q", phrase), true
			}
		}
	}

	return "", false
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
	const tag = el.tagName.toLowerCase();
	const type = (el.getAttribute('type') || '').toLowerCase();
	const name = el.getAttribute('name') || '';
	let label = '';
	if (el.id) {
		const esc = (window.CSS && CSS.escape) ? CSS.escape(el.id) : el.id;
		const l = document.querySelector('label[for="' + esc + '"]');
		if (l) label = l.textContent.trim();
	}
	if (!label) {
		const wrapping = el.closest('label');
		if (wrapping) label = wrapping.textContent.trim();
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
		if (legend) groupLabel = legend.textContent.trim();
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
					options = append(options, g.Label)
				}
			}

			label := f.GroupLabel
			if label == "" {
				label = humanizeName(f.Name)
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

		fields = append(fields, ports.FormField{
			Ref:      strconv.Itoa(f.index),
			Label:    f.Label,
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

// Close releases the browser context and stops the Playwright driver.
// Callers must only invoke this when nothing useful has been shown to the
// user yet — every other path leaves the window open deliberately.
func (w *PlaywrightWorker) Close(_ context.Context) error {
	if err := w.context.Close(); err != nil {
		return fmt.Errorf("failed to close browser context: %w", err)
	}
	if err := w.pw.Stop(); err != nil {
		return fmt.Errorf("failed to stop playwright: %w", err)
	}
	return nil
}
