# Billing

`billing` owns user payment protocols, checkout orchestration, payment recovery, shared payment display rules, and user-facing payment pages.

## Stable Public Contracts

- `paymentContracts.ts`: payment, order, plan, provider, checkout, and dashboard protocol types shared with administrator and subscription features.
- `paymentDisplay.ts`: currency normalization/formatting, order status display, refund eligibility, date display, and plan-validity wording.
- `paymentMethods.ts`: canonical visible-method normalization shared with administrator payment settings.
- `orderStatusBadge.ts`, `orderTable.ts`: narrow order-component exports.
- `paymentProviderDialog.ts`, `paymentProviderList.ts`: narrow provider-management component exports.
- `paymentStore.ts`: compatibility-safe public Store entry.

Other features must not import `billing/presentation/`. Add a specific public entry only when the shared contract is stable; do not create a general UI barrel.

`@/types/payment` and the old presentation formatter modules remain type/function-compatible forwarding paths while consumers migrate. New code uses the public owners above.

## Invariants

- Keep Stripe and other payment SDKs lazy until the corresponding payment flow starts.
- Preserve checkout-info loading, visible-method alias handling, order payloads, recovery storage, callback resume tokens, polling, and popup/redirect/QR decisions.
- User and administrator routes both load the shared `payment.*` vocabulary from the user locale scope; shared components must not depend on `admin.*` keys.
- Currency fallbacks, ISO normalization, zero-decimal currencies, legacy USD plan display, and plural validity-unit behavior must remain unchanged.

## Verification

```sh
pnpm exec vitest run src/features/billing
pnpm exec vitest run src/features/subscriptions/__tests__/SubscriptionPlanCard.spec.ts
pnpm exec vitest run src/core/i18n/__tests__/routeLocaleCoverage.spec.ts
pnpm run lint:check
pnpm run typecheck
```
