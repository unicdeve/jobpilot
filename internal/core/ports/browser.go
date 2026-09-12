package ports

import "context"

// FormFieldKind is the kind of control a BrowserWorker found on an
// application form.
type FormFieldKind string

// Form field kinds.
const (
	FormFieldKindText     FormFieldKind = "text"
	FormFieldKindTextarea FormFieldKind = "textarea"
	FormFieldKindSelect   FormFieldKind = "select"
	FormFieldKindCheckbox FormFieldKind = "checkbox"
	FormFieldKindRadio    FormFieldKind = "radio"
)

// FormField is one field a BrowserWorker extracted from an application
// form page.
type FormField struct {
	// Ref is opaque to callers: a reference the same BrowserWorker instance
	// can use later, within the same page session, to fill this exact
	// field via Fill. It is not guaranteed valid across a Close/renavigate.
	Ref      string
	Label    string
	Kind     FormFieldKind
	Required bool
	// Options holds the visible choice labels for FormFieldKindSelect and
	// FormFieldKindRadio; empty for other kinds.
	Options []string
}

// BrowserWorkerFactory launches a fresh BrowserWorker (its own browser
// window/page). It's a factory rather than a shared instance because each
// PrepareApplication run gets its own window left open for the user — one
// run's browser is never reused or overwritten by the next.
type BrowserWorkerFactory func() (BrowserWorker, error)

// BrowserWorker drives a single browser page to navigate to a job
// application, extract its form, and fill in approved answers, per
// docs/01_architecture/03_browser_worker.md. It never clicks the target
// site's own submit control — leaving that for the human, per Review
// Mode's explicit-approval requirement (docs/02_features/09_review_mode.md).
type BrowserWorker interface {
	Navigate(ctx context.Context, url string) error
	// DetectBlocker reports whether the current page shows a known
	// blocking condition (e.g. a CAPTCHA challenge). This is a best-effort
	// heuristic, not an anti-bot solver — architecture principle "never
	// bypass CAPTCHAs or anti-bot protections" means we only ever detect
	// and stop, never attempt to defeat one.
	DetectBlocker(ctx context.Context) (reason string, blocked bool)
	ExtractForm(ctx context.Context) ([]FormField, error)
	// ExtractContent returns the current page's raw HTML, for a caller that
	// wants to read the page itself (e.g. a job posting) rather than
	// extract a form.
	ExtractContent(ctx context.Context) (string, error)
	Fill(ctx context.Context, fieldRef, value string) error
	// Close releases the underlying browser resources. It must only be
	// called when nothing useful has been shown to the user yet (e.g. the
	// browser itself failed to launch) — on every other path the window is
	// deliberately left open for the user to review and submit themselves.
	Close(ctx context.Context) error
}
