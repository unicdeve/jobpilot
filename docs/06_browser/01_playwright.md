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
- CAPTCHA or a page that is a human check, which stops the import (ADR-0033 §1)
- unsupported form
- selector mismatch
- timeout
