# Sub2API Docker Image

Sub2API is an AI API Gateway Platform for distributing and managing AI product subscription API quotas.

## Quick Start

```bash
docker run -d \
  --name sub2api \
  -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/sub2api" \
  -e REDIS_URL="redis://host:6379" \
  ghcr.io/dr-lin-eng/sub2api-no2api:latest
```

## Docker Compose

```yaml
version: '3.8'

services:
  sub2api:
    image: ghcr.io/dr-lin-eng/sub2api-no2api:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://postgres:postgres@db:5432/sub2api?sslmode=disable
      - REDIS_URL=redis://redis:6379
    depends_on:
      - db
      - redis

  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=sub2api
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    mem_limit: 2g
    command: ["redis-server", "--save", "", "--appendonly", "no", "--maxmemory", "1536mb", "--maxmemory-policy", "allkeys-lru"]

volumes:
  postgres_data:
```

The example uses the small-machine Redis preset. See [Redis Tuning](./REDIS_TUNING.md) for memory sizing and the `50k+ RPM` preset.

## Startup and Database Recovery

Sub2API applies migrations during startup. PostgreSQL may still be recovering
briefly after a host or Docker daemon restart, so the application retries only
transient PostgreSQL startup and connection-class errors with bounded
exponential backoff. Invalid credentials, migration checksum mismatches, SQL
errors, and other permanent failures still stop startup immediately.

The full Compose health check verifies both `pg_isready` and a simple SQL query.
`depends_on: condition: service_healthy` orders a fresh Compose start, while the
application-level retry also covers restored containers after a daemon restart.

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Yes | - |
| `REDIS_URL` | Redis connection string | Yes | - |
| `PORT` | Server port | No | `8080` |
| `GIN_MODE` | Gin framework mode (`debug`/`release`) | No | `release` |

## Supported Architectures

- `linux/amd64`
- `linux/arm64`

## Tags

- `latest` - Latest stable release
- `x.y.z` - Specific version
- `x.y` - Latest patch of minor version
- `x` - Latest minor of major version

## Links

- [GitHub Repository](https://github.com/DR-lin-eng/sub2api-no2api)
- [Documentation](https://github.com/DR-lin-eng/sub2api-no2api#readme)
