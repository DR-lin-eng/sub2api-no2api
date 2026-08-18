# Auth

`auth` owns browser authentication, registration, OAuth completion, access-token restoration, and authentication state.

## Ownership

- `data/dtos/authDtos.ts`: auth, OAuth, captcha, invitation, and password-recovery protocol types.
- `data/datasources/authSessionActions.ts`: login, registration, 2FA login, logout, session refresh, session revocation, and in-memory token helpers.
- `data/datasources/authQueries.ts`: current-user, public-settings, and local-captcha reads.
- `data/datasources/authVerificationActions.ts`: email verification, promo/invitation validation, and password recovery.
- `data/datasources/authOAuthActions.ts`: OAuth start, pending completion, account creation/adoption, bind-token preparation, and WeChat capability selection.
- `data/datasources/authDatasource.ts`: compatibility facade only. New runtime code imports the owner above.
- `presentation/stores/authStore.ts`: authenticated user state, initial cookie restoration, refresh scheduling, and pending OAuth state.
- `index.ts`: stable `useAuthStore` entry for cross-feature consumers.
- `totpStepUpDialog.ts`: narrow public step-up component entry.

## Session Invariants

- Access tokens and any legacy response refresh token remain in `core/networks/tokenStore.ts` memory only.
- The durable browser refresh credential is an HttpOnly cookie. JavaScript must not persist it in Local Storage, Session Storage, IndexedDB, or a readable cookie.
- Initial protected-route checks await `authStore.checkAuth()` before evaluating user/admin permissions.
- Cached `auth_user` profile data is display-only. Administrator status remains false until a full auth response or `/auth/me` confirms the current role in this tab.
- Browser refresh goes through `core/networks/sessionRefresh.ts`, which deduplicates same-tab requests and serializes refresh-token rotation across tabs.
- A transient `/auth/me` failure after a successful cookie refresh does not discard the restored session, but cached roles cannot unlock admin routes, components, onboarding, or locale scopes; an authenticated 401 clears the session.
- OAuth completion keeps only the existing opaque pending-session summary needed to resume account creation/binding. Passwords and access/refresh token pairs remain memory/server owned.

## Dependency Rules

- Other features import `useAuthStore` from `@/features/auth` and step-up UI from `@/features/auth/totpStepUpDialog`.
- Other features must not import `auth/presentation/`.
- Auth/profile presentation must not import `@/api`, `@/stores`, or `authDatasource.ts`.

## Verification

```sh
pnpm exec vitest run src/features/auth src/features/profile
pnpm exec vitest run src/core/networks/__tests__/sessionRefresh.spec.ts
pnpm exec vitest run src/features/auth/__tests__/authProfileLocaleScopes.spec.ts
pnpm run lint:check
pnpm run typecheck
```
