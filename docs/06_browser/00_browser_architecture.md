# Browser Architecture

The browser reads a job page that no registered source can fetch, for import by URL, with Go Playwright behind `ports.BrowserWorker`.
It uses the user's installed Google Chrome, not Playwright's bundled browser.
A shared `ChromeSession` launches Chrome lazily in a dedicated JobPilot profile and gives each read its own tab, closed once the page is read.

It never fills in or submits an employer's form ([ADR-0034](../09_decisions/0034-the-candidate-writes-and-fills-their-own-application.md), [ADR-0014](../09_decisions/0014-scouts-and-no-unattended-submission.md)).
See [Browser worker architecture](../01_architecture/03_browser_worker.md) for what it does and how a human check is handled.
