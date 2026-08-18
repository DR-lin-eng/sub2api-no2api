# Admin Orders

`admin-orders` owns administrator payment configuration, dashboards, orders, refunds, channels, subscription plans, and provider-instance operations.

## Ownership

- `data/dtos/adminPaymentDtos.ts`: administrator-only request and response shapes.
- `data/datasources/adminPaymentQueries.ts`: read-only `/admin/payment/*` requests.
- `data/datasources/adminPaymentActions.ts`: configuration, order lifecycle, refund, channel, plan, and provider mutations.
- `data/datasources/adminPaymentDatasource.ts`: compatibility facade only; new runtime code must use the Query or Action owner directly.
- `presentation/`: administrator pages and widgets. Shared payment contracts and display rules come from the stable `billing` public files, never from `billing/presentation/`.

The shared `PaymentOrder`, `SubscriptionPlan`, provider, currency-aware dashboard, and checkout protocols are owned by `@/features/billing/paymentContracts`.

## Invariants

- Keep every existing `/admin/payment/*` path, query parameter, payload, and Axios response shape unchanged.
- Preserve dashboard stale-request rejection, order search debounce, refund pending/force handling, and write-then-refresh order.
- Do not import `adminPaymentAPI` from presentation code; it exists only for the top-level legacy admin API.
- Do not import another feature's private `presentation/` files. Use a narrow public component entry when domain UI must be shared.

## Verification

```sh
pnpm exec vitest run src/features/admin-orders
pnpm exec vitest run src/features/billing/__tests__/paymentLocaleScopes.spec.ts
pnpm run lint:check
pnpm run typecheck
```
