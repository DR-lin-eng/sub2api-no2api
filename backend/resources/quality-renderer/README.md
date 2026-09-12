# Quality renderer runtime dependencies

These packages are installed by the main Docker image at build time. The embedded
worker and model live in `internal/modules/qualityrender/assets` and are included
in the Go binary, not loaded from a remote service. No deployment settings change.
