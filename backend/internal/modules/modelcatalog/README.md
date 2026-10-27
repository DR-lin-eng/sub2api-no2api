# Model catalog

`modelcatalog` owns bounded provider-neutral model descriptors and versioned snapshots. Application services fetch through the selected account's existing transport, capture metadata from the same response, and persist complete successful catalogs through a narrow repository port. This module does not perform network or database calls.

`Descriptor` retains field presence, so explicit `false` and empty capability arrays survive normalization and fallback merging. Parsing accepts standard OpenAI/Anthropic lists, Codex manifests, Gemini lists and keyed Antigravity metadata. Limits are 2048 models, 256-byte IDs, 8 MiB across fetched pages and a 1 MiB stored snapshot. Failed, malformed or incomplete refreshes retain the previous successful snapshot. Successful snapshots remove models absent from the complete new catalog.

The `accounts.extra.upstream_model_metadata` snapshot uses version 1 and a refresh-start generation. Persistence compares account credentials, platform/type, proxy identity and generation. Ordinary account edits preserve the latest snapshot under the existing account row lock; changing the upstream identity removes it. Unknown future versions remain opaque and are not replaced by this version. Existing `UpdateExtra` writers merge their fields and preserve the new key. Migration `249_account_model_metadata_rolling_writes.sql` adds a narrow row trigger so older binaries replacing Extra from stale caches also preserve the current snapshot. It uses only OLD/NEW row values, keeps unknown future versions, rejects stale generations and clears snapshots on upstream identity changes. The additive guard remains installed during binary rollback; it does not rewrite existing rows.

Codex projection uses administrator `extra.model_metadata_overrides`, then explicit live upstream values, stored metadata, and compiled known-family defaults. The compiled JSON is generated from the reviewed repository price resource with `python3 backend/scripts/update-model-catalog.py` and currently includes 201 descriptors. Overrides are keyed by model ID, use the same validated descriptor fields, and may target an alias independently of its upstream target. Capability metadata never expands the account mapping or group allowlist. Projection copies the shared manifest and derives a body ETag after account mapping/metadata changes; serving a model manifest adds no per-request database or registry request.

Run focused tests from `backend/`:

```sh
go test ./internal/modules/modelcatalog ./internal/application/service -run 'Catalog|Claude55'
go test -tags=integration ./internal/infrastructure/repository -run TestModelCatalogPostgresRefreshAndLegacyWrites
```

Grok catalog diagnostics reuse this snapshot with optional `discovery_scope`; legacy snapshots remain unknown until refreshed. `grok.go` owns pure trust/mapping evaluation. Diagnostics never alter routing, pricing or model mappings. String-ID catalogs retain empty capability descriptors.
