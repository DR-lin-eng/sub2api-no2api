# Edge and HTTP Ingress Security

Sub2API supports long-lived SSE and WebSocket requests. Protect the request
ingress without imposing a response `WriteTimeout`: a write deadline would
terminate healthy long generations and streams.

## Application defaults

- `server.max_header_bytes: 65536` limits HTTP/1 request headers to 64 KiB;
  Go maps it to the corresponding HTTP/2 header-list limit.
- `server.read_header_timeout: 10` bounds slow-header attacks. It does not
  limit request processing or response streaming.
- `server.max_request_body_size: 268435456` is the absolute 256 MiB safety net.
- `gateway.max_body_size: 268435456` remains available to multimodal, Gemini,
  image, video, and batch-image endpoints.
- `gateway.text_max_body_size: 33554432` limits the known pure-text
  `/embeddings` and `/alpha/search` endpoints to 32 MiB.
- H2C defaults to 50 concurrent streams per connection, a 2 MiB connection
  upload window, and a 512 KiB stream upload window.
- Invalid credential abuse is limited in process by trusted client IP (IPv6
  `/64`): 120 failures per 60 seconds followed by a 60-second block. Optional
  Cloudflare integration can mirror each temporary block to either Zone IP
  Access Rules or sharded WAF Custom Rules before subsequent traffic reaches
  the origin.

## Cloudflare invalid-auth blocking

Open **Admin -> Risk Control -> Ingress Protection** and configure the
Cloudflare integration there. The Zone ID, enable switch, queue limits, and
timeouts are persisted in the database. The API token is encrypted with the
server's AES-GCM secret key and is never returned to the browser after save.
There is no YAML or environment-variable credential path. Enabling the
integration validates the selected Cloudflare resources before saving.

The request path only places a block event on a bounded in-memory queue, so
Cloudflare latency and errors do not delay the gateway request. IPv4 sources
use an exact IP; IPv6 sources use the same `/64` grouping as the local limiter.
Settings hot-apply on the instance that accepts the admin request; other
replicas refresh the same persisted snapshot within 10 seconds.

### Zone IP Access Rule mode

This mode creates, extends, reconciles, and removes one Zone IP Access Rule per
IP/CIDR. The token needs permission to read and edit Zone Firewall Services.
Managed rules carry a `sub2api-invalid-auth` note and an expiry timestamp.
Before removal, the worker reads the current remote expiry again so another
replica's extension is not deleted early.

Zone IP Access Rules affect the entire zone, not only the configured Sub2API
hostname, and they consume the zone's IP Access Rule quota.

### WAF Custom Rule mode

Create one or more enabled `block` rules in the zone's **Custom Rules** phase,
then paste their 32-character Rule IDs into the admin page. Sub2API takes full
ownership of those rules' expressions; do not add unrelated conditions to the
same rules. The token needs permission to read and edit the zone custom
ruleset. Add Zone Analytics read permission to display the cached request
statistics; a missing analytics permission does not stop expression syncing.

Set one or more exact hostnames served by Sub2API. A single hostname uses
`http.host eq "api.example.com"`; multiple hostnames use an inline set such as
`http.host in {"api.example.com" "edge.example.com"}`. Entries are automatically
split across the configured rules without exceeding Cloudflare's 4,096-byte
expression limit. IP expiry state is stored in Redis, and the existing
distributed leader lock ensures that only one replica patches the rules at a
time. All configured hostnames must belong to the selected zone.

New IPs are coalesced for the configurable WAF sync interval (15 seconds by
default). An unchanged desired expression does not call Cloudflare; a full
remote comparison still runs at the configured reconcile interval. Aggregate
and per-hostname request totals plus managed-rule block events for the last 24
hours are fetched in one grouped GraphQL request, cached in Redis, and refreshed
every 300 seconds by default. Refreshing the admin page only reads this cache.

Keep the default threshold conservative when users may share NAT egress
addresses. The security client IP must also be trustworthy: restrict origin
access to Cloudflare or a private proxy and configure `server.trusted_proxies`
correctly. If clients can bypass Cloudflare and reach the origin directly, edge
blocking does not reduce origin load.

To disable the integration without leaving temporary rules behind, turn off
**Enable edge blocking** in the Ingress Protection page. The worker enters
cleanup-only mode and stops accepting new blocks. Mode, Zone ID, token,
hostname, and WAF Rule ID replacement remain locked until the page reports zero
queued work and zero active entries, preventing a binding change from orphaning
temporary blocks.

Do not add a single application-wide request semaphore: an SSE request may
legitimately occupy it for many minutes. Apply connection and unauthenticated
request controls at the edge; authenticated user/API-key concurrency remains
the application's responsibility.

## Trusted client IPs

Sub2API defaults to the `auto_compat` client-IP mode. Loopback/private peers,
Cloudflare's embedded official ranges, and explicit trusted proxies may supply
forwarded client addresses. Public non-Cloudflare peers cannot override their
TCP peer address with forwarding headers. This supports common Nginx, Caddy,
Docker, Cloudflare orange-cloud, and Cloudflare Tunnel deployments without a
configuration-file change.

`server.trusted_proxies` is an optional additional list of CIDR/IP addresses
that connect directly to Sub2API. It is merged with proxy entries configured in
the admin settings. Use the `trusted_proxy` mode in the admin settings when
private infrastructure ranges should not be trusted automatically, or `direct`
to ignore all forwarding headers.

Never trust `CF-Connecting-IP`, `X-Real-IP`, or `X-Forwarded-For` merely because
the header exists. A CDN deployment must firewall the origin so only the CDN or
load balancer can reach it, and the proxy must overwrite forwarded headers.

Example for a proxy on the same host:

```yaml
server:
  trusted_proxies:
    - 127.0.0.1/32
    - ::1/128
```

## Nginx baseline

Define shared zones in the `http` block. Tune rates to measured legitimate
traffic; the values below are conservative starting points, not universal
capacity targets.

```nginx
limit_conn_zone $binary_remote_addr zone=sub2api_conn:20m;
limit_req_zone  $binary_remote_addr zone=sub2api_auth:20m rate=5r/s;
limit_req_zone  $binary_remote_addr zone=sub2api_api:40m rate=30r/s;
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;

    client_header_timeout 10s;
    client_max_body_size 256m;
    large_client_header_buffers 4 16k;
    limit_conn sub2api_conn 40;

    location ~ ^/(auth|api/auth)/ {
        limit_req zone=sub2api_auth burst=10 nodelay;
        proxy_pass http://127.0.0.1:8080;
    }

    location ~ ^/(v1/)?(embeddings|alpha/search)$ {
        client_max_body_size 32m;
        limit_req zone=sub2api_api burst=60 nodelay;
        proxy_pass http://127.0.0.1:8080;
    }

    location / {
        limit_req zone=sub2api_api burst=60 nodelay;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 1800s;
        proxy_send_timeout 1800s;
        proxy_pass http://127.0.0.1:8080;
    }
}
```

If Nginx gzip is enabled in the `http` block, keep `text/event-stream` out of
`gzip_types` and do not use `gzip_types *` for Sub2API. The
`proxy_buffering off` setting above prevents proxy buffering, but it does not
disable the gzip response filter. Use an explicit list for ordinary responses:

```nginx
gzip on;
gzip_types text/plain text/css application/json application/javascript application/xml image/svg+xml;
```

If a shared global configuration cannot exclude SSE by content type, set
`gzip off;` in the locations serving streaming API routes. This leaves gzip
available for the web UI and static assets.

Do not use an incoming `$http_x_forwarded_for` value unless Nginx real-IP
processing is restricted to explicit trusted proxy CIDRs.

## Caddy and CDN

The bundled `deploy/Caddyfile` sets a 64 KiB header limit, a 10-second header
timeout, a 256 MiB absolute body limit, and overwrites forwarded addresses from
the TCP peer. It is therefore a direct-to-Caddy baseline. Do not use its
`{remote_host}` forwarding lines unchanged behind a CDN: all clients would be
attributed to a CDN egress address, collapsing rejection aggregation and the
invalid-auth limiter onto unrelated users.

The bundled Caddy configuration leaves `flush_interval` unset so Caddy can
automatically flush `text/event-stream` responses while still propagating
client cancellation upstream. Do not set it globally: positive values can add
streaming latency, while Caddy 2.6.2's special `-1` mode also causes
reverse-proxied requests to continue after clients disconnect. The
configuration uses an explicit response content-type list for compression. Do
not replace that list with `text/*` or the shorthand `encode gzip zstd`: both
match `text/event-stream` and can buffer SSE until the response ends. Keep
streaming responses uncompressed while retaining compression for the web UI,
JSON, and static assets.

For a CDN deployment, first firewall the origin so only current CDN egress
CIDRs can connect. Then configure those exact ranges as Caddy trusted proxies
and derive upstream headers from Caddy's parsed `{client_ip}`. For example:

```caddyfile
{
	servers {
		trusted_proxies static 192.0.2.0/24 2001:db8:1234::/48
		trusted_proxies_strict
		client_ip_headers CF-Connecting-IP X-Forwarded-For
	}
}

api.example.com {
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {client_ip}
		header_up X-Forwarded-For {client_ip}
	}
}
```

Replace the documentation ranges with the CDN's published, automatically
maintained egress ranges. `CF-Connecting-IP` is safe here only because direct
origin access is blocked and Caddy trusts only those TCP peers. Sub2API's
default automatic mode recognizes a local/private Caddy peer; public custom
proxy addresses can be added in the admin settings or `server.trusted_proxies`.

Caddy core does not provide a general request-rate limiter; use a trusted
CDN/WAF, a supported rate-limit module, or host firewall controls.

At a CDN/WAF, configure connection limits, header/body limits, bot challenges,
and per-IP/ASN rates before traffic reaches the origin. Allow origin ingress
only from CDN egress CIDRs or a private load balancer. Keep the application port
off the public Internet.

## DDoS boundary

Application checks reduce amplification after a connection reaches Go. They
cannot absorb volumetric attacks, TLS floods, bandwidth saturation, or a large
distributed source set. Those require upstream network capacity, CDN/WAF
filtering, provider firewall rules, and origin isolation. Avoid high-cardinality
metrics or per-request database security logs during rejection storms.
