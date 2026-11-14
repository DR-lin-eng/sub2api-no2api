# ChatGPT App images

Pure request, integrity protocol and stream decoding for the 20261107 ChatGPT App Images workflow.
The application adapter owns credentials, account egress, uploads, downloads,
SSE/topic recovery, API projection and reliable billing. This module performs no I/O.

`integrity*` implements the 25-slot prepare key, native Go PoW and the bounded
register VM recovered in the report's 2026-11-08 supplement. The VM uses an
embedded pure Go JavaScript interpreter only when requested. It has no network,
filesystem, process or account credential bindings. Environment data is a small
protocol model, not a real browser or device attestation; the persisted account
device ID is supplied by the application, separately from each prepare's UUID.

PoW reuses JSON/base64 buffers and hashes UTF-16 seed code units. Validation
work is limited to four concurrent solvers (or GOMAXPROCS if lower), 500,000
nonces and five seconds including admission. The VM has a 500 ms logical timer,
a 550 ms hard interrupt, and instruction, queue, register, nesting and value
limits. Unsupported host APIs are not synthesized. Exhaustion/rejection fails
before conversation dispatch; successful headers retain the source's mutually
exclusive token/prepare-token precedence. Golden fixtures are generated from
the exact supplied module by `diagnostics/chatgpt-integrity-20261108/reference-fixtures.cjs`.

The App conversation model and its internal image model are separate identities.
Input attachments and unfinished image placeholders are never returned as generated images.
