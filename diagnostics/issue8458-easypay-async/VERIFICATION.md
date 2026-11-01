# Issue #8458 and optional EasyPay asynchronous payment verification

Validation base: `0a3513a9f3d18402718be94d1591eb342af86af5`.

## Final behavior

- [#8458](https://github.com/Wei-Shaw/sub2api/issues/8458): both Chat Completions handlers reject negative effective output token limits with HTTP 400 before routing. Non-null `max_completion_tokens` retains precedence, including zero. Internal converters are unchanged.
- [#8443](https://github.com/Wei-Shaw/sub2api/issues/8443): an explicit **Async payment** (`async`) option is added beside the existing **QR Code** (`qrcode`) and **Synchronous redirect (popup)** (`popup`) options. Async uses server-side `mapi.php`; popup retains browser-side `submit.php`. QR and legacy `api`, `redirect`, and empty values retain their existing API behavior.
- Existing payment modes are preserved by API reads, ordinary edits, and instance selection. The admin dialog preserves recognized legacy values when saving unrelated fields. Administrators can explicitly select async and switch back to popup.
- No database migration is included; existing provider configurations and payment modes are preserved during upgrades.
- Payment signatures, same-instance upstream confirmation, callback retries, and idempotent fulfillment use the existing implementation. V2 RSA-only gateways require a V1-compatible endpoint.

## Validation environment

Backend Docker image: `sub2api-backend-test:upstream-sync-20261030` (Go 1.26.10), with current workspace sources mounted at `/app/backend`.

Frontend Docker image: `sub2api-pr169-frontend:review`, with current `frontend/src` and lint configuration mounted read-only. Its dependency manifest and lockfile exactly match the workspace:

```text
package.json   1e4c3c48a38f7f593c25718f16ad283c5b38db22626268ec04ea423c7d868a3f
pnpm-lock.yaml e6bdb04f5266fc940d20a75b3dba7d28a533550b416539c8828f5b280885cded
```

## Final compatibility validation

The final compatibility tests exercise desktop/mobile async requests, legacy popup checkout with zero server-side API calls, unchanged legacy modes, explicit bidirectional configuration changes, async failures without a silent protocol fallback, and callback retry/idempotency without browser return.

Commands:

```sh
go test -p 1 -tags=unit ./internal/modules/payment/... ./internal/application/service \
  -run 'Payment|EasyPay|Provider|BuildSelection' -count=1
pnpm exec vitest run src/features/billing \
  src/features/admin-settings/__tests__/settings.paymentVisibleMethods.spec.ts \
  src/features/admin-settings/__tests__/SettingsPage.spec.ts \
  src/core/i18n/__tests__/routeLocaleCoverage.spec.ts \
  src/__tests__/dynamicLocaleMapping.spec.ts
pnpm run typecheck
pnpm run lint:check
```

Results on the final compatible implementation:

- Backend payment module: passed (`0.005s`). Provider suite: passed (`0.016s`). Application payment suite: passed (`3.419s`), including the explicit mode persistence matrix and async notification retry/idempotency test.
- Frontend: **227/227 tests passed across 28 files**, including ordinary-save preservation of six mode values, explicit async/popup switching, desktop/mobile launch decisions, admin settings, and both locale gates.
- TypeScript typecheck and ESLint: passed.
- `git diff --check`: passed.

## Compatible production image and upgrade

`docker build -t sub2api:issue8458-easypay-compatible .` passed, including the production frontend and backend with embedded assets. Linux/ARM64 image ID:

```text
sha256:68f4589338400ca4a1d89ea346f2b18b71cea679f64bdd1eeeb4172cb1b5b84e
```

A dedicated disposable Compose project (`sub2api-8458-qa`) started the exact baseline image `ghcr.io/dr-lin-eng/sub2api-no2api:sha-0a3513a` with PostgreSQL 18 and Redis 8. The app was published only on `127.0.0.1:18458`.

Five disabled EasyPay fixtures covered empty, `api`, `popup`, `qrcode`, and `redirect` modes, plus one official Alipay control. After baseline config encryption, snapshots captured provider type, payment mode, ciphertext, enablement, and every migration filename/checksum.

Upgrading the same database to the compatible image returned `{"ready":true}`. **All six provider rows matched the snapshot across all compared fields**, including each original mode. **Migration-record changes: zero** (313 unchanged records). This confirms that upgrades preserve synchronous checkout and require an explicit administrator choice to switch.

The replacement image also passed the 12 real HTTP #8458 cases, plus readiness and embedded-frontend checks. No upstream account was configured and no real payment occurred.

The disposable test containers and network were removed after verification. The final compatible image is retained locally.

The unchanged #8458 handler implementation previously passed the complete HTTP handler unit suite (`81.513s`), complete protocol-compatibility suite (`1.010s`), and 12 real HTTP cases covering both platform groups, both stream values, and field precedence. Those results apply to the unchanged token-validation paths, not to the revised payment option.

## Existing repository gates

- `settingsGatewayResilienceModularization.spec.ts`: unrelated template hash mismatch, reproduced in the untouched baseline Docker image.
- `make check-docs`: unmodified `docs/README.md` links to absent `PR169_MEDIA_STUDIO_REVIEW.md`.
- `make -C backend check-layout`: unmodified `payment_fulfillment_test.go` (1201 lines) and `pricing_service.go` (1218 lines) exceed the 1200-line gate. Neither file nor the allowlist was changed.

Payment validation uses synthetic merchant credentials and local HTTP fixtures; no real payment or production deployment was performed.
