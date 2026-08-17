# Admin IPv6 Egress

This feature owns the administrator workflow for account-scoped IPv6 egress:
runtime readiness, prefix pools, account bindings, route selection, rotation,
source-address probes, and the container-only HE sidecar control surface.

- `data/datasources/adminEgressDatasource.ts`: backend protocol and typed API calls.
- `presentation/pages/EgressPage.vue`: page-level loading, pagination, selection,
  dialogs, step-up actions, and mutations.

The page reuses the account datasource only for the account list. All egress
mutations remain owned by this feature. The backend is the security boundary;
frontend availability checks only improve the administrator experience.
