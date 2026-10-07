# Browser Architecture

Browser preparation is implemented with Go Playwright behind `ports.BrowserWorker`.
It uses the user's installed Google Chrome, not Playwright's bundled browser.
A shared `ChromeSession` launches Chrome lazily in a dedicated JobPilot profile and gives each task its own tab.

The worker extracts and fills supported fields, records activity and leaves the browser open for human review and submission.
It never clicks an external submit control (ADR-0014).
See [Browser worker architecture](../01_architecture/03_browser_worker.md) for navigation, validation and failure behavior.
