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

// ApplyControl is a link or button on a job posting that might lead to the
// application form. The worker reports what is on the page; which one to
// follow is decided in core (services.PickApplyControl), so the rule is
// testable without a browser and identical for every connector.
type ApplyControl struct {
	// Ref is opaque to callers: a reference the same BrowserWorker instance
	// can use with FollowControl, valid until the page changes.
	Ref string
	// Text is the control's visible text.
	Text string
	// URL is the resolved destination, empty for a button that navigates
	// through script rather than an href.
	URL string
}

// BrowserWorkerFactory returns a BrowserWorker on its own fresh page. It's
// a factory rather than a shared instance because each PrepareApplication
// run gets its own tab left open for the user — one run's page is never
// reused or overwritten by the next. The browser behind those pages is
// shared, since Chrome permits only one instance per profile directory.
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
	// and wait for the user, never attempt to defeat one (ADR-0033).
	DetectBlocker(ctx context.Context) (reason string, blocked bool)
	// Focus brings this worker's page to the front, for a check only the
	// user can solve.
	Focus(ctx context.Context) error
	// PageLoads is how many documents this worker's page has finished
	// loading. It means nothing on its own: core compares two readings to
	// tell whether the page reloaded in between.
	PageLoads(ctx context.Context) int
	ExtractForm(ctx context.Context) ([]FormField, error)
	// CurrentURL is where the page has ended up, which is not necessarily
	// where it was sent: an apply link commonly redirects to the employer's
	// applicant tracking system on another origin.
	CurrentURL(ctx context.Context) string
	// FindApplyControls lists the page's visible links and buttons, for
	// core to choose between. It filters nothing itself.
	FindApplyControls(ctx context.Context) ([]ApplyControl, error)
	// FollowControl clicks one and waits for the page to settle.
	FollowControl(ctx context.Context, ref string) error
	// ExtractContent returns the current page's raw HTML, for a caller that
	// wants to read the page itself (e.g. a job posting) rather than
	// extract a form.
	ExtractContent(ctx context.Context) (string, error)
	Fill(ctx context.Context, fieldRef, value string) error
	// Close closes this worker's own page, leaving the browser and any
	// other run's page untouched. It must only be called when nothing
	// useful has been shown to the user yet — on every other path the page
	// is deliberately left open for the user to review and submit
	// themselves.
	Close(ctx context.Context) error
}
