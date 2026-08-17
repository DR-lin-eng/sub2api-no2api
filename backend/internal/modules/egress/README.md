# IPv6 Egress Module

This module owns IPv6 egress pools, deterministic account bindings, rotation,
and the management use cases around them. The low-level route value and Linux
dialer live in `internal/platform/egress`; persistence is implemented by
`internal/infrastructure/repository/egress_repo.go`.

The HE control service validates frontend tunnel settings and writes a bounded
desired-state/action protocol through `HETunnelControlStore`. Its filesystem
implementation is infrastructure-owned; only the dedicated container sidecar
has network-management capabilities, and secret update keys are never returned
through the management API.

Account gateway requests must carry the binding already loaded with the account.
They must not query this module from the steady-state request path.

Every enabled process runs its own default-pool route preflight. Only a
worker-enabled process reconciles missing inherited-account bindings. Pool
health is intentionally process-local and is cleared when a pool is disabled;
the persistent binding and default-pool facts remain in PostgreSQL.
