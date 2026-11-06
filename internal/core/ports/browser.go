package ports

import "context"

// BrowserWorkerFactory returns a BrowserWorker on its own fresh page. The
// browser behind those pages is shared, since Chrome permits only one
// instance per profile directory.
type BrowserWorkerFactory func() (BrowserWorker, error)

// BrowserWorker drives a single browser page to read a job posting that no
// registered source recognizes (JobDiscoveryService.ImportJob), per
// docs/01_architecture/03_browser_worker.md.
//
// It reads and never acts: JobPilot does not fill or submit an employer's
// form (ADR-0034 §2).
type BrowserWorker interface {
	Navigate(ctx context.Context, url string) error
	// DetectBlocker reports whether the current page shows a known
	// blocking condition, such as a page that is itself a human check
	// (ADR-0033 §1). It only detects: JobPilot never gets past an
	// anti-bot measure.
	DetectBlocker(ctx context.Context) (reason string, blocked bool)
	// ExtractContent returns the current page's raw HTML.
	ExtractContent(ctx context.Context) (string, error)
	// Close closes this worker's own page, leaving the browser and any
	// other worker's page untouched.
	Close(ctx context.Context) error
}
