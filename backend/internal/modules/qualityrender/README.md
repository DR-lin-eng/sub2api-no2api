# Embedded quality renderer

`Processor.Process` invokes the embedded `assets/worker.py` using a local Python subprocess, without HTTP or a listening port. The main Docker image includes the Python/Playwright/Chromium CPU runtime at build time. Existing Compose files, environment and persisted settings are unchanged. Explicit legacy renderer URLs still use their original HTTP path. Source-code grading is performed by the Go fingerprint matcher.

Each call uses a private temporary directory and a minimal environment without app credentials. HTML is limited to 1 MiB, output to 12 MiB, PNG/WebP to 4 MiB each. Only one local renderer runs per process; cancellation kills its process group on Unix, and rendering has a 90-second deadline. The browser uses CSP, offline mode, request blocking and a fresh context; raw HTML/SVG is never served.

The renderer only produces PNG and animated WebP previews. The Go fingerprint matcher scores the complete HTML/SVG source using 9 weighted signals (default threshold 55); the score is a similarity indicator, not a probability. Native binary installations need Python 3.11, Pillow, Playwright and Chromium; Docker includes these with no deployment edits.

Run module tests from backend: `go test ./internal/modules/qualityrender`. Use `QUALITY_RENDER_INTEGRATION=1` for the real browser preview test inside the packaged runtime. It writes PNG/WebP only when an output directory is supplied by the test environment.
