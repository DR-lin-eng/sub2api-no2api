# Multi-Instance Deployment

All replicas must use the same PostgreSQL database and Redis database. Keep
`JWT_SECRET`, `TOTP_ENCRYPTION_KEY`, database credentials, and Redis credentials
identical across replicas. Explicit secret injection remains the preferred
production setup.

Configure every replica explicitly:

```yaml
deployment:
  mode: multi_instance
  node_id_file: /app/data/.cluster-node-id
  node_name: api-01
  worker_enabled: auto
  heartbeat_interval_seconds: 30
  stale_after_seconds: 90
  task_lease_seconds: 60
  rollout_poll_seconds: 5
  # Retained for configuration compatibility; rollouts no longer wait for a
  # pre-restart drain.
  rollout_drain_grace_seconds: 10
  rollout_drain_timeout_seconds: 900
  rollout_verify_heartbeats: 2
```

Every replica continues to serve the complete API and embedded frontend.
`worker_enabled` only controls cluster-wide scheduled work. `auto` and `true`
join lease-based worker election; `false` keeps the node API/frontend-only.

## Stable node identity and names

A logical node has three separate identifiers:

- `node_id` is the durable identity used by rollout targets. When it is not
  configured explicitly, Sub2API creates it once in `node_id_file`.
- `node_name` is only the initial display name. An administrator can rename a
  node from `/admin/multi-instance`; the PostgreSQL alias survives restarts and
  later changes to the container hostname. If two new nodes use the same
  initial name, the later node receives a stable ID suffix so both can register
  and then be renamed from the page.
- `runner_id` identifies one process lifetime. It changes on every restart and
  is retained only as history.

The node table shows only the newest runner for each `node_id`. Recreating a
container or upgrading its image therefore does not add another visible node as
long as the node identity file is preserved.

For Docker, mount `/app/data` on a node-local persistent volume. Preserve that
same volume when replacing the container on the same machine. Every replica
must have its own volume: sharing one `/app/data/.cluster-node-id` between two
live replicas makes them the same logical node and is unsupported. When cloning
a machine to add capacity, assign a new explicit `DEPLOYMENT_NODE_ID` or start
the clone with a fresh node-local identity file.

## Cluster release workflow

All release plans and per-node targets are stored in the shared PostgreSQL
database. Consequently, an administrator connected to any replica sees every
logical node and its version, can rename any node, and can create or control the
same rollout. The request does not depend on the load balancer routing it to a
specific target node.

A rollout resolves `latest` to one exact stable version before creating its
targets. The candidate version is persisted before the first node downloads a
binary. Only one plan can be active, and targets advance in a fixed order with
`max_unavailable=1`:

```text
pending -> installing -> restarting -> verifying -> succeeded
                                             -> awaiting_confirmation -> completed
```

A failure pauses the plan. Retry the failed target to continue, or cancel only
when no target is actively installing, restarting, or verifying. Once every
target has verified the candidate, the plan waits in
`awaiting_confirmation`; the administrator must explicitly confirm it before
the version becomes locked. Multi-instance mode rejects the legacy local
update, rollback, and restart endpoints with `MULTI_INSTANCE_ROLLOUT_REQUIRED`.

Each selected node downloads the exact signed release and exits like the
standalone updater. There is no pre-restart grace period or in-flight request
wait. During an active plan the readiness gate accepts both the source and
candidate versions, so old and new nodes may coexist briefly; the external
load balancer should continue routing to healthy replicas and tolerate the
normal restart interruption of the selected node. Docker deployments must keep
`/app` writable and use a restart policy such as `unless-stopped`; the supplied
images and Compose files already do both.

The rollout changes the running container's writable layer, not its image
reference. Recreating that container from an older image can therefore restore
an older binary and make `/ready` return `503`. Before recreating a node, ensure
the selected image is at least the cluster's desired version.

`/health` remains a liveness probe. `/ready` is the load-balancer and Compose
readiness probe. During an active rollout it accepts the source/candidate pair;
after manual confirmation it enforces the locked version and returns `503` for
any other version. Keep nodes with dependency failures or an unverified
version out of request traffic.

Run the disposable two-node Docker scenario from the repository root:

```sh
sh deploy/tests/cluster-rollout-docker-test.sh
```

When validating an unreleased state-machine change, replace nodes with the
locally built target image instead of downloading the latest published binary:

```sh
CLUSTER_TEST_IMAGE_REPLACEMENT=1 sh deploy/tests/cluster-rollout-docker-test.sh
```

`CLUSTER_TEST_NO_CACHE=1` forces clean image builds, and
`CLUSTER_TEST_GOPROXY` overrides the Go module proxy for that test only.

The scenario builds distinct source and target application images, starts two
replicas with separate persistent identity volumes and shared PostgreSQL/Redis,
renames and recreates a node, creates a rollout through the other replica,
rolls both nodes in order, confirms the verified candidate, and verifies
version convergence plus `/ready` rejection after an old image is reintroduced.
Resources are removed on exit.

## Installation and secrets

`config.yaml` and `.installed` remain local files. With `AUTO_SETUP=true`, the
first replica holds a PostgreSQL advisory lock while it migrates the database,
creates the initial administrator, persists the cluster JWT secret, and writes a
database installation marker. Other replicas then adopt that installation and
only write their local files.

At normal startup PostgreSQL is authoritative for both the JWT signing secret and
the TOTP encryption key. The first persisted value wins; a replica configured
with a different value logs a warning and uses the persisted value. Treat the
`security_secrets` table as sensitive backup data.

## OAuth and cache state

Claude, OpenAI, Gemini, Antigravity, and xAI/Grok authorization sessions are
stored in Redis with a 30-minute TTL. An authorization started on one replica can
therefore be exchanged on another replica. Redis failures are returned as an
authorization service error instead of being misreported as an expired session.

Do not configure Redis with an eviction policy that aggressively removes these
short-lived keys. Monitor keys under `oauth:session:*` and ensure the Redis memory
limit leaves headroom for request, billing, scheduler, and OAuth state.

## Background jobs

Scheduled account tests, scheduled backups, and channel monitor checks use
renewable PostgreSQL task leases. The active node renews its lease every third of
the configured lease duration. A node that loses the lease has its task context
canceled and cannot record a successful completion. Stale leases are marked lost
before another worker takes over. Redis/PostgreSQL leader locks remain as a
compatibility fallback when the cluster coordinator is not injected.
Finished task history is retained for seven days and capped at 10,000 rows;
runner history is retained for seven days. Historical runners do not increase
the logical node count.

Each heartbeat also stores one current runtime-load snapshot for that node. The
snapshot includes normalized CPU and memory usage, accepted in-flight requests,
active background tasks, goroutine count, and active/idle/maximum PostgreSQL and
Redis pool connections. Container deployments read CPU and memory from cgroups;
other environments fall back to process metrics. Sampling failures leave the
affected CPU or memory value unavailable instead of reporting a false zero.

The admin page at `/admin/multi-instance` puts these load snapshots first, with
cluster averages and peaks plus sortable per-node cards. It also shows node
heartbeats, dependency health, editable node names, version distribution,
rollout targets, resolved worker mode, active leases, and recent task history.
Load data is a latest-heartbeat snapshot rather than a historical time series.
Nodes running an older binary remain visible and show no load metrics until
they are upgraded; telemetry that falls behind continuing heartbeats is hidden.

## Admin metrics quick reference / 管理页指标速查

The following table explains the summary cards at the top of
`/admin/multi-instance`. The Chinese names are the labels shown by the Chinese
admin interface.

| UI label | Meaning |
| --- | --- |
| 在线节点 | Online logical nodes divided by all visible logical nodes. The detail line separates stale and stopped nodes. |
| 平均 CPU | Mean CPU usage of online nodes that reported a valid CPU sample. The detail line shows the highest node value and the number of reporting nodes. Container values are normalized to the cgroup CPU quota; non-container deployments use normalized process CPU. |
| 平均内存 | Mean memory percentage of online nodes for which a usable memory limit is known. The detail line shows the highest node percentage and reporting-node count. |
| 处理中请求 | Sum of `in_flight_requests` from online nodes that have a current load snapshot. It counts requests accepted by the node that have not returned from their Gin handler yet; it is not a request rate. |
| 活跃任务 | Cluster-wide count of currently running leased background tasks, such as scheduled account tests, backups, and channel checks. It is separate from HTTP requests and from the usage-billing queue backlog. |
| 异常节点 | Online nodes whose latest heartbeat reports PostgreSQL or Redis unhealthy. Stale and stopped nodes are reported separately rather than counted here. |

Each node card uses the following fields:

| UI label | Meaning |
| --- | --- |
| CPU | CPU used by this process/container relative to its available CPU capacity. |
| 内存 | Current process/container memory usage. The byte value remains useful even when a percentage cannot be calculated. |
| 处理中请求 | Requests currently owned by this node's accepted-request counter. Business APIs, admin/user APIs, gateway requests, and long-lived streams can all contribute. |
| 活跃任务 | Running leased background tasks currently owned by this runner. |
| Goroutine | Result of Go's `runtime.NumGoroutine()` for this process. It includes request handlers, connection-pool helpers, background loops, timers, and other runtime work; it is not the user count, request count, or operating-system thread count. |
| 运行时间 | Time since this runner process started. It resets when the process restarts. |
| PostgreSQL | Active connections divided by this node's configured pool maximum; the second line is the idle connection count. |
| Redis | Active connections divided by this node's configured pool maximum; the second line is the idle connection count. |

### Sampling and counting boundaries

- These values are the latest heartbeat snapshot, not a live stream or a
  historical chart. With the default configuration, a heartbeat is sent every
  30 seconds, so short spikes may occur between snapshots.
- `处理中请求` increments only after a multi-instance node accepts the request
  through its readiness gate and decrements when the handler finishes.
  SSE, streaming responses, and WebSocket handlers may therefore keep it above
  zero for the lifetime of the connection. `/health`, `/ready`, and
  `/api/v1/admin/cluster` (including its sub-routes) are excluded so probes and
  this page's polling do not affect rollout state.
- A displayed request count of `0` means no accepted handler was still running
  at the latest sample. It does not mean that the node received no traffic
  during the heartbeat interval. The counter is telemetry only; rollouts no
  longer wait for it to reach zero before installation.
- Billing settlement, cleanup workers, timers, and other background work do not
  increase `处理中请求`. Leased scheduled work is shown separately as
  `活跃任务`; billing-queue backlog and throughput use their dedicated metrics.
- CPU and memory averages include only online nodes that supplied that metric.
  Check the reporting-node count and per-node cards before treating an average
  as cluster-wide coverage.
- A memory percentage of `-` together with a value such as `550.1 MB` means the
  current usage was measured but no usable memory limit was available. It does
  not mean memory usage is zero.
- PostgreSQL and Redis pool maxima are per-node limits. Use the aggregate
  formulas in [Capacity calculation](#capacity-calculation) when sizing shared
  services.

### Reading common situations

- `处理中请求 0`: at the last heartbeat, that node had no accepted HTTP handler
  waiting to finish. Background settlement or scheduled work may still be
  running.
- `Goroutine 289`: the Go process had 289 goroutines at that sample. There is no
  universal failure threshold; compare the idle baseline and trend. A count
  that grows across multiple heartbeats and does not fall after traffic drains
  deserves investigation, while a stable count can be normal for a busy node.
- `处理中请求` stays above zero across several heartbeats: first check for
  legitimate long-lived SSE/WebSocket sessions, then inspect access/gateway
  logs for a slow or stuck handler. This can delay a rolling release drain.
- PostgreSQL or Redis `active / max` remains close to the maximum while latency
  or pool waits rise: inspect the affected node and the aggregate cluster pool
  size before increasing limits. A high Redis idle count alone is not pool
  exhaustion.
- Average CPU looks normal but the displayed peak is high: inspect the per-node
  cards because the average can hide a hot replica or uneven load balancing.
- Goroutines, in-flight requests, active tasks, and pool usage rising together
  usually indicate real load. Goroutines rising alone and never returning to
  baseline is a stronger signal of a blocked or leaked background path than a
  single absolute value.

Do not point replicas at different Redis databases. That would split the lock and
OAuth state domains and restore duplicate execution and callback misses.

## WebSocket load balancing

The load balancer must preserve HTTP/1.1 upgrade headers and must not impose a
short idle or response timeout on WebSocket routes. A connection stays on the
replica that accepted it. OpenAI `response_id -> account_id` and session turn
state are Redis-authoritative when Redis is configured. Local maps are used only
without Redis, so a binding deleted by one replica cannot remain live in another
replica's L1 cache. Shared turn-state keys use
`openai:ws:state:turn:v1:<group_id>:<session_hash_digest>`.

Connection IDs and live WebSocket connections remain process-local because a
connection ID only identifies a socket in the accepting replica's connection
pool. A continuation routed to another replica can recover account routing and
turn metadata from Redis, but it may need a new upstream connection. Connection
affinity is therefore still recommended for reconnect and continuation traffic.

Example Nginx baseline:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

location / {
    proxy_pass http://sub2api_pool;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

Caddy handles WebSocket upgrades automatically. Configure an affinity policy at
the external load balancer when continuation requests must follow the original
connection-owning replica.

## Capacity calculation

Database and Redis pool settings are per replica. Size the cluster using:

```text
postgres_connections = replicas * database.max_open_conns + reserved_connections
redis_clients         = replicas * redis.pool_size + non_application_clients
```

`deployment.mode: multi_instance` also keeps user and API-key concurrency slots in Redis
so limits and live counts are cluster-wide. `standalone` uses process-local atomic
slots instead, avoiding a single Redis sorted-set hot key when one API key carries
very high concurrency.

Reserve PostgreSQL connections for migrations, administration, backup tooling,
and incident access. For example, four replicas at `max_open_conns: 64` require
at least 256 application slots plus the reserve, not four pools of 256 unless
PostgreSQL is explicitly sized for more than 1024 application connections.

Set `max_idle_conns` no higher than `max_open_conns`, then validate actual pool
waits before increasing it. Redis `maxclients`, memory, file descriptor limits,
and load-balancer upstream connection limits must cover the aggregate replica
count. Keep HTTP streaming and WebSocket timeouts longer than the longest allowed
upstream request.
