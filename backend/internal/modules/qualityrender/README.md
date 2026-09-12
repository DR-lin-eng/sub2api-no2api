# Embedded quality renderer

`Processor.Process` invokes the embedded `assets/worker.py` and checksum-pinned
`assets/best.pt` using a local Python subprocess, without HTTP or a listening port.
The main Docker image includes the Python/Playwright/Chromium/Ultralytics 8.4.14 CPU
runtime at build time. Existing Compose files, environment and persisted settings
are unchanged. Explicit legacy renderer URLs still use their original HTTP path.

Each call uses a private temporary directory and a minimal environment without app
credentials. HTML is limited to 1 MiB, output to 12 MiB, PNG/WebP to 4 MiB each.
Only one local renderer runs per process; cancellation kills its process group on
Unix, and rendering/classification has a 90-second deadline. The browser uses CSP,
offline mode, request blocking and a fresh context; raw HTML/SVG is never served.

The actual model grades sampled frames (0, 7, 15). SVG presence alone never yields
a pass. Native binary installations need Python 3.11 and the packages in
`../../../resources/quality-renderer/requirements.txt`, torch 2.6.0 CPU and
Chromium installed by Playwright; Docker includes these with no deployment edits.

Run module tests from backend: `go test ./internal/modules/qualityrender`.
Use `QUALITY_RENDER_INTEGRATION=1` for the real browser/model test inside the
packaged runtime. It writes a PNG/WebP only when an output directory is supplied
by the test environment.
