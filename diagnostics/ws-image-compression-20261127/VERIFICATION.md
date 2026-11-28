# Verification

2026-11-27 Docker verification passed on Linux arm64 with `golang:1.26.11-alpine`, 2 CPU, 4 GiB memory, `GOMAXPROCS=2`, `CGO_ENABLED=0` and `--network none`.

- Platform config defaults and validation passed.
- API Key budget override and API Key compression switch passed. OAuth budget resolution remained fixed at 128 MiB despite custom legacy values.
- OpenAI WS service tests passed, including 132 MiB read limits, 32/64/128 MiB hard-limit routing, image compression, Alpha preservation and stringified tool output.
- HTTP handler WS size and streaming recovery matrix passed.
- Pure-Go Linux arm64 release build passed in Docker with `CGO_ENABLED=0`.
- Compression benchmark, three iterations, measured `131.0 ms/op`, `36.0 MB/op` and `306 allocs/op` for a 1024×1024 noisy PNG fixture. This is a bounded single-request baseline under two CPUs, not an end-to-end TTFT measurement.

The fixture uses local fake transports and does not establish the production OpenAI upstream hard limit or model-level visual quality.
