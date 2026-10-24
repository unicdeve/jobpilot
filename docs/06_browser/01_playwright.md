# Playwright

## Requirements

- Go Playwright
- User-installed Google Chrome
- Cross-platform operation
- Explicit browser lifecycle
- Cancellation support

## Failure handling

Return structured errors for:
- browser unavailable
- page unavailable
- navigation failure
- authentication required
- CAPTCHA, which the run waits for the user to answer (ADR-0033)
- unsupported form
- selector mismatch
- timeout
