# Profile

`profile` owns current-user profile editing, password changes, balance notifications, identity binding, and TOTP setup/disable UI.

## Ownership

- `data/datasources/profileDatasource.ts`: profile, password, notification-email, identity-binding, affiliate, and current-user quota requests.
- `data/datasources/totpDatasource.ts`: TOTP status, setup, enable, disable, verification, and step-up requests.
- `presentation/`: `/profile` page and profile-owned widgets.

Profile presentation imports named datasource functions directly. `userAPI` and `totpAPI` remain compatibility objects for `src/api/index.ts` only.

Cross-feature state and UI use stable public entries:

- Auth Store: `@/features/auth`
- Passkey card: `@/features/passkeys/profilePasskeyCard`

## Invariants

- Profile refresh updates the non-sensitive cached user snapshot but never stores access or refresh tokens.
- Identity binding prepares the short-lived server cookie through the auth OAuth owner before leaving the page.
- Password, notification-email, avatar, TOTP, and identity mutations preserve their existing payloads and write-then-refresh order.
- Shared auth/profile components must resolve their text from the locale scopes actually loaded by public, user, and administrator routes.

## Verification

```sh
pnpm exec vitest run src/features/profile
pnpm exec vitest run src/features/auth/__tests__/authProfileModularization.spec.ts
pnpm exec vitest run src/features/auth/__tests__/authProfileLocaleScopes.spec.ts
```
