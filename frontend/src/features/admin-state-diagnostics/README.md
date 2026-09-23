# Admin State Diagnostics

State Diagnostics owns `/admin/state-diagnostics`, a read-only view of the
current node's redacted Codex Turn State observations joined with the OpenAI
OAuth account pool. It intentionally sits beside account operations rather
than inside gateway settings.

- `data/datasources/stateDiagnosticsQueries.ts`: current-node diagnostics and
  the complete OpenAI OAuth pool query.
- `data/dtos/stateDiagnosticsDtos.ts`: account-row and filter contracts.
- `presentation/composables/stateDiagnosticsTransforms.ts`: pure grouping,
  health classification, and filtering rules.
- `presentation/pages/StateDiagnosticsPage.vue`: refresh, filtering, summary,
  pool distribution, and account table.

The backend only returns redacted state metadata. An account with no observed
model remains visible as `unobserved`, while inactive or unschedulable accounts
are shown as `blocked`; the page never treats a missing observation as a
healthy state.

## Verification

From `frontend/`:

```sh
pnpm exec vitest run src/features/admin-state-diagnostics
pnpm run typecheck
```
