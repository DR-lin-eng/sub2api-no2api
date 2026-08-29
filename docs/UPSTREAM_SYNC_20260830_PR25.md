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
| `21fa95b0`, `61921400` support chat rewrite/text | Superseded and excluded | Current `main` already owns database quick replies, structured messages, recall, protected assets, unread/history recovery, and their security tests. The PR deleted those tests and regressed the current contract. |
| `2f4b0623` TLS provider wiring | Overlap resolved | Only the constructor consolidation is retained. Current account-level stable/diverse TLS profiles and HTTP/1.1 transport behavior remain authoritative. |
| `5eb407e0` access token in URL | Excluded | The existing origin-checked `postMessage` delegation remains authoritative; query-string credentials would leak through URL history, logs, and referrers. |
| `7cfcc32e` request-template literals/tests | Included | Kept with the validated template contract. |

## Upgrade and performance boundaries

- The feature switch defaults to disabled, so existing model routing does not change after upgrade.
- Migrations continue after existing migration `233`; no duplicate migration prefix or checksum replacement is introduced.
- Every pre-upgrade API key named `Media Studio` keeps its token but receives a deterministic legacy display name. Only newly managed keys can receive Media Studio privileges.
- Custom model exact and longest-prefix lookup uses a five-second compiled snapshot. CRUD invalidates the snapshot immediately; transient refresh failures retain the last successful snapshot.
- Browser capability data is loaded only when an administrator opens a consuming workflow. Anonymous and ordinary application startup adds no admin request.
- Multi-image generation sends one `n` request first and creates bounded `n=1` fallback requests only when an upstream returns fewer images than requested.
- Routing middleware shares its buffered video body with the selected handler, avoiding duplicate body reads on composite/custom video paths.

## Verification to retain

- Docker frontend lint, typecheck, and production build passed with Node 24 and pnpm 11.17.0. Full Vitest passed 346 files and 2,124 tests.
- Docker Go unit tests passed every package; the largest application and handler owners completed in 175.428s and 31.099s. golangci-lint v2.9 reported `0 issues`.
- The integration harness ran inside Go 1.26.6 with Docker CLI/daemon 29.2.1 and created PostgreSQL 18.1 plus Redis 8.4 Testcontainers. Repository integration passed in 12.165s.
- Cached capability lookup measured 64.81-65.37 ns/op, 0 B/op, and 0 allocs/op across five benchmark runs.
- The exact `origin/main` baseline image was `sha256:d0896727...da7da252`; the candidate image was `sha256:0959cfe9...77d8cdd5`. Candidate size was 42,376,438 bytes.
- A persisted baseline database upgraded from 284 to 287 migrations. Migration 233 retained checksum `d6e18505...87b0bfea`; migrations 234-236, both tables, case-insensitive indexes, and the `ON DELETE SET NULL` template foreign key were present after restart.
- A scratch upgrade with two pre-existing `Media Studio` keys preserved `sk-old-1` and `sk-old-2`, renamed both displays to deterministic legacy names, and created the managed-key unique index.
- In-app browser verification covered feature-toggle persistence, template creation, prefix capability creation, Media Studio empty state, and group-route model empty state. Desktop and 390px viewports had no horizontal overflow.

## Existing gate repairs

The frozen `main` frontend baseline had three failures before integration: stale quota-test dates, two account modules above the 1,500-line gate, and stale hashes for settings cards changed by later mainline commits. The integration branch repaired those gates without changing request ownership:

- Passive quota behavior remains fail-closed after a reset; the fixture now uses relative future reset timestamps.
- Bulk capacity fields and pure account transforms moved to static same-feature owners. Route chunks and state/request timing remain unchanged.
- Codex and 429 settings cards remain on the shared page context; obsolete computed state was removed and audited template hashes were refreshed.

Production bundle comparison against exact `origin/main` kept the initial entry effectively flat. Accounts changed from 808.23 kB/175.41 kB gzip to 809.37 kB/175.66 kB gzip; Settings changed from 463.74 kB/94.95 kB to 465.11 kB/95.11 kB. Media Studio grew from 30.71 kB/10.01 kB to 58.26 kB/17.84 kB, and Custom Model Config is a separate lazy 29.94 kB/7.38 kB chunk.

Future syncs should treat every row above as reviewed. Reopen a row only when the upstream behavior or the current owner materially changes.
