# Quality code matching

The account-quality stage 2 grader is a pure Go HTML/SVG source matcher in
`fingerprint.go`. It evaluates nine weighted signals from the supplied
`model_a_fingerprint.py` reference and records score, threshold, matched and
missing signals. the frontend renders the bounded preview HTML in a sandboxed iframe, so the backend image does not include Python, Chromium or Playwright.
