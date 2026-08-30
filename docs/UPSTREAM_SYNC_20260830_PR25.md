# PR #25 selective integration review (2026-08-30)

This record closes the semantic review of repository PR #25 at original head
`7cfcc32ea5193edaa26aad6a0968da68d571819c`. The PR was based on
`9d84842e7c6ff16c50960372b81f67bda5028e31`; integration was rebuilt on the
current modular `main` instead of merging the stale tree unchanged.

## Decision table

| Original commit | Decision | Current owner or reason |
| --- | --- | --- |
| `aed4d42f` custom model capabilities | Reworked and included | Admin CRUD now enters `transport -> application -> repository`; gateway reads a compiled, invalidatable snapshot instead of querying PostgreSQL per request. |
| `c6ebb8be` video account-test guard | Included | Account-test UI keeps video models out of the unsupported generic test path. |
| `9fcc6942` OpenAI-format Media Studio routing | Reworked and included | Group routes and managed API keys preserve current authorization, billing, and feature-owner boundaries. |
| `e62df7d9` request templates | Hardened and included | Templates have bounded JSON, relative-path, content-type, body-mode, variable, and protected-header validation. |
| `bf79e21e` image attachments and selection | Included with fixes | Retry keeps source files, object URLs are lifecycle-safe, and concurrent fallback results cannot overwrite each other. |
| `6ea9973f` Media Studio menu consolidation | Included | Retained within the existing Media Studio feature owner. |
| `f090d0aa` Agnes video | Hardened and included | Task ownership remains scoped to user and API key; content uses the bound account's fixed content endpoint and never follows an upstream-supplied arbitrary URL. |
| `21fa95b0`, `61921400` support chat rewrite/text | Semantically integrated in the follow-up | Retained current database quick replies, structured messages, recall, protected assets, unread/history recovery, and security tests; restored explicit cleanup opt-in, ID/content search, per-conversation drafts, boundary-correct multi-image composition, built-in/one-click replies, expanded emoji, image preview, and quote navigation. PR HTML rendering and public asset URLs remain excluded. |
| `2f4b0623` TLS provider wiring | Overlap resolved | Only the constructor consolidation is retained. Current account-level stable/diverse TLS profiles and HTTP/1.1 transport behavior remain authoritative. |
| `5eb407e0` access token in URL | URL credentials excluded; safe capability flow included | URLs still strip credentials. The per-menu switch now issues a 90-second, menu/origin-scoped capability through origin/source-checked `postMessage`; a derived signing key prevents the capability from authenticating to normal Sub2API endpoints. |
| `7cfcc32e` request-template literals/tests | Included | Kept with the validated template contract. |

## Upgrade and performance boundaries

- The feature switch defaults to disabled, so existing model routing does not change after upgrade.
- Migrations continue after existing migration `233`; no duplicate migration prefix or checksum replacement is introduced.
- Every pre-upgrade API key named `Media Studio` keeps its token but receives a deterministic legacy display name. Only newly managed keys can receive Media Studio privileges.
- Custom model exact and longest-prefix lookup uses a five-second compiled snapshot. CRUD invalidates the snapshot immediately; transient refresh failures retain the last successful snapshot.
- Browser capability data is loaded only when an administrator opens a consuming workflow. Anonymous and ordinary application startup adds no admin request.
- Multi-image generation starts one independent `n=1` request per requested image concurrently, bounded by the existing maximum count of four. Partial failures retain successful results in request order.
- Routing middleware shares its buffered video body with the selected handler, avoiding duplicate body reads on composite/custom video paths.

## Verification to retain

- Docker frontend lint, typecheck, and production build passed with Node 24 and pnpm 11.17.0. Final full Vitest passed 347 files and 2,129 tests.
- Docker Go 1.26.6 unit tests passed every package serially; the application service owner completed in 178.263s. golangci-lint v2.9 reported `0 issues`.
- The integration harness ran inside Go 1.26.6 with Docker CLI/daemon 29.2.1 and created PostgreSQL 18.1 plus Redis 8.4 Testcontainers. Final chat repository integration passed in 4.766s.
- Cached capability lookup measured 64.81-65.37 ns/op, 0 B/op, and 0 allocs/op across five benchmark runs.
- The follow-up candidate image was built as `sub2api-pr25-followup:review` with local manifest `sha256:513a5a843d49f06f1d717558312b28274851baa8e974ae32b2a2ee4e1138029b`; candidate size was 43,344,292 bytes.
- A persisted baseline database upgraded from 284 to 287 migrations. Migration 233 retained checksum `d6e18505...87b0bfea`; migrations 234-236, both tables, case-insensitive indexes, and the `ON DELETE SET NULL` template foreign key were present after restart.
- A scratch upgrade with two pre-existing `Media Studio` keys preserved `sk-old-1` and `sk-old-2`, renamed both displays to deterministic legacy names, and created the managed-key unique index.
- In-app browser verification covered feature-toggle persistence, template creation, prefix capability creation, Media Studio empty state, group-route model empty state, admin multi-image upload, user image upload, and authenticated image preview. Desktop and 390px viewports had no horizontal overflow.

## Existing gate repairs

The frozen `main` frontend baseline had three failures before integration: stale quota-test dates, two account modules above the 1,500-line gate, and stale hashes for settings cards changed by later mainline commits. The integration branch repaired those gates without changing request ownership:

- Passive quota behavior remains fail-closed after a reset; the fixture now uses relative future reset timestamps.
- Bulk capacity fields and pure account transforms moved to static same-feature owners. Route chunks and state/request timing remain unchanged.
- Codex and 429 settings cards remain on the shared page context; obsolete computed state was removed and audited template hashes were refreshed.

Production bundle comparison against exact `origin/main` kept the initial entry effectively flat. Accounts changed from 808.23 kB/175.41 kB gzip to 809.37 kB/175.66 kB gzip; Settings changed from 463.74 kB/94.95 kB to 465.11 kB/95.11 kB. Media Studio grew from 30.71 kB/10.01 kB to 58.26 kB/17.84 kB, and Custom Model Config is a separate lazy 29.94 kB/7.38 kB chunk.

## Follow-up closure

The follow-up was rebased onto `main` at `b95c2f8b8d412c1b25a2442c43f388093eec6e35` and closes the deferred support/media/token behavior without reviving the unsafe original implementations:

- Media Studio now starts one independent `n=1` request for every requested image concurrently, with a hard maximum of four and partial-success preservation.
- Support chat keeps protected structured assets while restoring multi-image preview/send, per-conversation drafts, built-in and one-click replies, expanded emoji, image preview, quote navigation, user/admin history pagination, and admin search by identity, exact ID, or non-recalled message text. Numeric ID searches do not scan message bodies.
- The original `Image file is required` browser failure was traced to the shared Axios JSON default overriding FormData. Support uploads now remove that default so the browser supplies a valid multipart boundary. Browser verification sent two admin images and one user image; both asset endpoints, the structured message writes, and authenticated image reads returned `200`.
- Message cleanup now requires the separately persisted `support_chat_retention_enabled` switch. Missing or false retains everything; the worker re-reads the policy between bounded batches so an administrator can stop or change a running cleanup cycle.
- The legacy per-menu `forward_access_token` switch now sends only a 90-second menu/origin capability. Exact-origin `postMessage` plus source-checked ready handling is single-flight. Live introspection returned `200` with `custom_menu:access`, while using the same token against `/api/v1/auth/me` returned `401`.
- Final Node 24 Docker Vitest passed 347 files and 2,128 tests. Docker typecheck, lint, production build, Go 1.26.6 unit tests, PostgreSQL 18.1/Redis 8.4 Testcontainers chat integration, and golangci-lint v2.9 also passed. Browser checks covered desktop and 390 px layouts without horizontal overflow.

Future syncs should treat every row above as reviewed. Reopen a row only when the upstream behavior or the current owner materially changes.
