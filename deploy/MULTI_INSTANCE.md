# Multi-Instance Deployment

All replicas must use the same PostgreSQL database and Redis database. Keep
`JWT_SECRET`, `TOTP_ENCRYPTION_KEY`, database credentials, and Redis credentials
identical across replicas. Explicit secret injection remains the preferred
production setup.

Configure every replica explicitly:

```yaml
deployment:
  mode: multi_instance
  node_name: api-01
  worker_enabled: auto
  heartbeat_interval_seconds: 30
  stale_after_seconds: 90
  task_lease_seconds: 60
```

Every replica continues to serve the complete API and embedded frontend.
`worker_enabled` only controls cluster-wide scheduled work. `auto` and `true`
join lease-based worker election; `false` keeps the node API/frontend-only.
Use a stable, unique `node_name` per replica.

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
stopped node records are retained for seven days.

The admin page at `/admin/multi-instance` shows node heartbeats, dependency
health, resolved worker mode, active leases, and recent task history.

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
