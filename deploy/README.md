# Sub2API Deployment Files

This directory contains files for deploying Sub2API on Linux servers and Apple-silicon Macs.

## Deployment Methods

| Method | Best For | Setup Wizard |
|--------|----------|--------------|
| **Docker Compose** | Quick setup, all-in-one | Not needed (auto-setup) |
| **Apple container** | Native local stack on macOS 26 | Not needed (auto-setup) |
| **Binary Install** | Production servers, systemd | Web-based wizard |

## Files

| File | Description |
|------|-------------|
| `docker-compose.yml` | Docker Compose configuration (named volumes) |
| `docker-compose.local.yml` | Docker Compose configuration (local directories, easy migration) |
| `docker-compose.ipv6-egress.yml` | Linux override for account-scoped IPv6 egress |
| `ipv6-egress-host.sh` | Host route and forwarding preflight for the IPv6 pool |
| `docker-compose.ipv6-egress-he.yml` | Container-only Hurricane Electric 6in4 sidecar override |
| `Dockerfile.ipv6-egress-sidecar` | Minimal image for the HE network sidecar |
| `he-ipv6-tunnel.sh` | Idempotent 6in4 tunnel lifecycle used inside the sidecar |
| `ipv6-egress-sidecar.sh` | Frontend control queue and container-network route agent |
| `ipv6-egress-env.sh` | Non-evaluating allowlist parser for IPv6 control files |
| `docker-deploy.sh` | **One-click Docker deployment script (recommended)** |
| `apple-container.sh` | Native Apple `container` lifecycle script |
| `APPLE_CONTAINER.md` | Apple `container` deployment and operations guide |
| `.env.example` | Container environment variables template |
| `DOCKER.md` | Docker Hub documentation |
| `REDIS_TUNING.md` | Redis memory sizing and 50k+ RPM preset |
| `MULTI_INSTANCE.md` | Multi-instance secrets, workers, admin metrics reference, WebSocket, and capacity guidance |
| `install.sh` | One-click binary installation script |
| `install-datamanagementd.sh` | datamanagementd 一键安装脚本 |
| `sub2api.service` | Systemd service unit file |
| `sub2api-datamanagementd.service` | datamanagementd systemd service unit file |
| `DATAMANAGEMENTD_CN.md` | datamanagementd 部署与联动说明（中文） |
| `config.example.yaml` | Example configuration file |
| `EDGE_SECURITY.md` | Reverse proxy, CDN/WAF, trusted proxy, and ingress hardening guide |

---

## Apple container Deployment

Apple-silicon Macs running macOS 26 can run the complete Sub2API, PostgreSQL, and Redis stack with Apple `container` 1.1.0 or newer:

```bash
./apple-container.sh init
./apple-container.sh up
./apple-container.sh status
./apple-container.sh logs app -f
```

The script uses Apple named volumes, starts dependencies in order, and performs live readiness checks. It does not provide a continuous restart supervisor; run `./apple-container.sh up` after a host reboot. Docker Compose remains the recommended production deployment path.

See [APPLE_CONTAINER.md](./APPLE_CONTAINER.md) for configuration, upgrades, persistence, networking behavior, and limitations.

---

## Docker Deployment (Recommended)

### Method 1: One-Click Deployment (Recommended)

Use the automated preparation script for the easiest setup:

```bash
# Download and run the preparation script
curl -fsSL https://raw.githubusercontent.com/DR-lin-eng/sub2api-no2api/main/deploy/docker-deploy.sh -o docker-deploy.sh && chmod 700 docker-deploy.sh
# Review the downloaded preparation script before running it.
bash ./docker-deploy.sh

# Or download first, then run
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/DR-lin-eng/sub2api-no2api/main/deploy/docker-deploy.sh -o docker-deploy.sh
chmod +x docker-deploy.sh
./docker-deploy.sh
```

**What the script does:**
- Downloads `docker-compose.local.yml` and `.env.example`
- Automatically generates secure secrets (JWT_SECRET, TOTP_ENCRYPTION_KEY, POSTGRES_PASSWORD)
- Creates `.env` file with generated secrets
- Creates necessary persistent data directories (data/, postgres_data/)
- **Displays generated credentials** (POSTGRES_PASSWORD, JWT_SECRET, etc.)

**After running the script:**
```bash
# Start services
docker compose -f docker-compose.local.yml up -d

# View logs
docker compose -f docker-compose.local.yml logs -f sub2api

# If admin password was auto-generated, find it in logs:
docker compose -f docker-compose.local.yml logs sub2api | grep "admin password"

# Access Web UI
# http://localhost:8080
```

### Method 2: Manual Deployment

If you prefer manual control:

```bash
# Clone repository
git clone https://github.com/DR-lin-eng/sub2api-no2api.git sub2api
cd sub2api/deploy

# Configure environment
cp .env.example .env
chmod 600 .env
nano .env  # Set POSTGRES_PASSWORD and other required variables

# Generate secure secrets (recommended)
JWT_SECRET=$(openssl rand -hex 32)
TOTP_ENCRYPTION_KEY=$(openssl rand -hex 32)
echo "JWT_SECRET=${JWT_SECRET}" >> .env
echo "TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY}" >> .env

# Create data directories
mkdir -p data postgres_data

# Start all services using local directory version
docker compose -f docker-compose.local.yml up -d

# View logs (check for auto-generated admin password)
docker compose -f docker-compose.local.yml logs -f sub2api

# Access Web UI
# http://localhost:8080
```

### Deployment Version Comparison

| Version | Data Storage | Migration | Best For |
|---------|-------------|-----------|----------|
| **docker-compose.local.yml** | Local directories (./data, ./postgres_data); Redis is volatile | ✅ Easy (tar persistent directories) | Production, need frequent backups/migration |
| **docker-compose.yml** | Named volumes (/var/lib/docker/volumes/) | ⚠️ Requires docker commands | Simple setup, don't need migration |

**Recommendation:** Use `docker-compose.local.yml` (deployed by `docker-deploy.sh`) for easier data management and migration.

### How Auto-Setup Works

When using Docker Compose with `AUTO_SETUP=true`:

1. On first run, the system automatically:
   - Connects to PostgreSQL and Redis
   - Applies database migrations (SQL files in `backend/migrations/*.sql`) and records them in `schema_migrations`
   - Persists a cluster-wide JWT secret in PostgreSQL (if not provided)
   - Creates admin account (password auto-generated if not provided)
   - Writes the local config.yaml and installation marker

   Concurrent replicas serialize this first-install sequence with a PostgreSQL
   advisory lock. Once the database installation marker exists, later replicas
   adopt it and only materialize their local files.

2. No manual Setup Wizard needed - just configure `.env` and start

3. If `ADMIN_PASSWORD` is not set, check logs for the generated password:
   ```bash
   docker compose logs sub2api | grep "admin password"
   ```

### Database Migration Notes (PostgreSQL)

- Migrations are applied in lexicographic order (e.g. `001_...sql`, `002_...sql`).
- `schema_migrations` tracks applied migrations (filename + checksum).
- Migrations are forward-only; rollback requires a DB backup restore or a manual compensating SQL script.

**Verify `users.allowed_groups` → `user_allowed_groups` backfill**

During the incremental GORM→Ent migration, `users.allowed_groups` (legacy `BIGINT[]`) is being replaced by a normalized join table `user_allowed_groups(user_id, group_id)`.

Run this query to compare the legacy data vs the join table:

```sql
WITH old_pairs AS (
  SELECT DISTINCT u.id AS user_id, x.group_id
  FROM users u
  CROSS JOIN LATERAL unnest(u.allowed_groups) AS x(group_id)
  WHERE u.allowed_groups IS NOT NULL
)
SELECT
  (SELECT COUNT(*) FROM old_pairs)           AS old_pair_count,
  (SELECT COUNT(*) FROM user_allowed_groups) AS new_pair_count;
```

### datamanagementd（数据管理）联动

如需启用管理后台“数据管理”功能，请额外部署宿主机 `datamanagementd`：

- 主进程固定探测 `/tmp/sub2api-datamanagement.sock`
- Docker 场景下需把宿主机 Socket 挂载到容器内同路径
- 详细步骤见：`deploy/DATAMANAGEMENTD_CN.md`

### Commands

For **local directory version** (docker-compose.local.yml):

```bash
# Start services
docker compose -f docker-compose.local.yml up -d

# Stop services
docker compose -f docker-compose.local.yml down

# View logs
docker compose -f docker-compose.local.yml logs -f sub2api

# Restart Sub2API only
docker compose -f docker-compose.local.yml restart sub2api

# Update to latest version
docker compose -f docker-compose.local.yml pull
docker compose -f docker-compose.local.yml up -d

# Remove all data (caution!)
docker compose -f docker-compose.local.yml down
rm -rf data/ postgres_data/
```

For **named volumes version** (docker-compose.yml):

```bash
# Start services
docker compose up -d

# Stop services
docker compose down

# View logs
docker compose logs -f sub2api

# Restart Sub2API only
docker compose restart sub2api

# Update to latest version
docker compose pull
docker compose up -d

# Remove all data (caution!)
docker compose down -v
```

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `POSTGRES_PASSWORD` | **Yes** | - | PostgreSQL password |
| `JWT_SECRET` | **Recommended** | *(auto-generated)* | JWT secret (fixed for persistent sessions) |
| `TOTP_ENCRYPTION_KEY` | **Recommended** | *(auto-generated)* | TOTP encryption key (fixed for persistent 2FA) |
| `SERVER_PORT` | No | `8080` | Server port |
| `ADMIN_EMAIL` | No | `admin@sub2api.local` | Admin email |
| `ADMIN_PASSWORD` | No | *(auto-generated)* | Admin password |
| `TZ` | No | `Asia/Shanghai` | Timezone |
| `DEPLOYMENT_MODE` | No | `standalone` | Set `multi_instance` on every load-balanced replica. |
| `DEPLOYMENT_NODE_ID` | No | *(generated)* | Optional fixed logical node identity. |
| `DEPLOYMENT_NODE_ID_FILE` | No | `/app/data/.cluster-node-id` | Node-local persistent identity file; never share it between replicas. |
| `NODE_NAME` | No | hostname | Initial display name; it can later be changed in the multi-instance admin page. |
| `UPDATE_GITHUB_TOKEN` | No | *(empty)* | Token for `api.github.com` release checks only; asset downloads remain anonymous. |
| `GEMINI_OAUTH_CLIENT_ID` | No | *(builtin)* | Google OAuth client ID (Gemini OAuth). Leave empty to use the built-in Gemini CLI client. |
| `GEMINI_OAUTH_CLIENT_SECRET` | No | *(builtin)* | Google OAuth client secret (Gemini OAuth). Leave empty to use the built-in Gemini CLI client. |
| `GEMINI_OAUTH_SCOPES` | No | *(default)* | OAuth scopes (Gemini OAuth) |
| `GEMINI_QUOTA_POLICY` | No | *(empty)* | JSON overrides for Gemini local quota simulation (Code Assist only). |
| `IPV6_EGRESS_ALLOCATION_SECRET` | No | *(auto-generated)* | Optional legacy bootstrap secret; the admin page generates and persists one when empty. |
| `IPV6_EGRESS_ENABLED` | No | `false` | Optional legacy bootstrap switch; use the IPv6 Egress admin page for the live switch. |
| `IPV6_EGRESS_POOL_CIDR` | Host setup | *(empty)* | Provider-routed global prefix passed to `ipv6-egress-host.sh`. |
| `IPV6_EGRESS_CONTAINER_IP` | No | `fd42:5355:4232::10` | Fixed Docker ULA next hop for the routed account pool. |
| `IPV6_EGRESS_CONTAINER_NAME` | No | `sub2api` | Running application container whose network namespace receives the pool local route. |
| `IPV6_EGRESS_CONTROL_AGENT_STALE_SECONDS` | No | `15` | Frontend sidecar heartbeat timeout. |
| `IPV6_EGRESS_CONTROL_POLL_SECONDS` | No | `2` | Sidecar control-queue polling interval. |
| `IPV6_EGRESS_CONTROL_VOLUME` | No | `sub2api-ipv6-egress-control` | Shared desired-state/status volume. |
| `HE_TUNNEL_INTERFACE` | No | `he-sub2api` | SIT interface name inside the shared application network namespace. |

See `.env.example` for all available options.

### Account-scoped IPv6 egress (Linux only)

This mode requires a prefix routed to the Docker host by the provider or by a
Hurricane Electric Tunnel Broker tunnel. A single IPv6 address, an ordinary
SLAAC address, or an IPv6 proxy endpoint is not sufficient. Use `standalone`
deployment mode; multi-instance routing is intentionally rejected until
node-owned prefixes and account affinity exist.

Use exactly one network override:

- `docker-compose.ipv6-egress.yml` for a provider prefix already routed to the host.
- `docker-compose.ipv6-egress-he.yml` for a container-only HE 6in4 tunnel.

#### Native routed prefix

1. Ensure the host/provider has routed IPv6 and create the stack. No IPv6 switch,
   secret, or pool CIDR is required in `.env`:

   ```dotenv
   DEPLOYMENT_MODE=standalone
   ```

2. Create the stack and its stable IPv6 Docker network:

   ```bash
   docker compose \
     -f docker-compose.local.yml \
     -f docker-compose.ipv6-egress.yml \
     up -d
   ```

3. Route the provider prefix to the application's fixed ULA next hop and add
   the pool as a local route in that container network namespace, then verify
   forwarding and container capabilities. The host needs `iproute2`, `sysctl`,
   `ping`, Docker, and `util-linux` (`nsenter`). Pass `.env` as the second
   argument; the parser imports only host IPv6 keys and never evaluates the
   file or loads application/database secrets:

   ```bash
   sudo ./ipv6-egress-host.sh apply .env
   sudo ./ipv6-egress-host.sh check .env
   ```

4. Open **Admin -> IPv6 Egress**, turn on the runtime switch, and click
   **Auto-detect and configure**. The page probes the prefix visible inside the
   application container and creates the default pool for you. Manual pool
   creation remains available for operators with multiple routed prefixes.

The Docker network ULA is only a next hop. Account source addresses are kept in
PostgreSQL and bound per socket with `IPV6_FREEBIND`; the main application has
no `NET_ADMIN`. The host script also installs `local <pool> dev lo` inside the
application network namespace so response packets reach those free-bound
sockets. The host must restore both routes after Docker recreates the container
or bridge, or after a reboot. Persist `net.ipv6.conf.all.forwarding=1` through
the host's normal sysctl management.

#### Container-only Hurricane Electric sidecar

Create a regular tunnel at [Hurricane Electric Tunnel Broker](https://tunnelbroker.net/),
then start the application with the HE override:

```bash
docker compose \
  -f docker-compose.local.yml \
  -f docker-compose.ipv6-egress-he.yml \
  up -d --build
```

Open **Admin -> IPv6 Egress -> HE Tunnel** to save and apply the tunnel, then
turn on the runtime switch and use **Auto-detect and configure**. The
main application remains unprivileged. A separate sidecar joins the
application's network namespace and is the only container with `NET_ADMIN` and
`NET_RAW`; it does not use host networking, the host PID namespace, the Docker
socket, or host filesystem mounts.

Open **Admin -> IPv6 Egress**, turn on the runtime switch, and click **Auto-detect
and configure**. The application inspects its own network namespace, verifies
the detected source through the IPv6 probe, creates a default pool, and stores
the deterministic allocation secret in PostgreSQL. If the container only has a
ULA or the host route is missing, the page reports that directly.

Copy the four distinct values from the HE tunnel detail page:

| HE field | Frontend field | Purpose |
| --- | --- | --- |
| Server IPv4 Address | HE Server IPv4 | Remote 6in4 endpoint |
| Client IPv6 Address | HE Client IPv6 /64 | Tunnel interface address |
| Server IPv6 Address | HE Server IPv6 | IPv6 default-route gateway |
| Routed /64 | HE Routed /64 or /48 | Account source-address pool |

The frontend can also configure HE dynamic endpoint updates using the tunnel
ID, account username, and tunnel **update key**. The key is stored only in the
shared control volume and is never returned by the API.

6in4 still requires the Docker host's public IPv4 path and upstream firewall to
carry IP protocol 41. Ordinary TCP/UDP port forwarding and CGNAT cannot provide
that transport. The sidecar uses the container IPv4 as its local endpoint and
keeps protocol 41 traffic active through Docker NAT; if the host or provider
drops protocol 41, the frontend check fails closed with the sidecar error.

IPv6 mode is fail closed. Missing AAAA records, an unavailable prefix, a bind
failure, or a route failure returns an upstream error and never falls back to
the server IPv4. Existing `proxy_id` accounts continue using their external
proxy. Turn off the runtime switch on the IPv6 Egress page to roll inherited
accounts back to direct routing; explicit `ipv6_pool` accounts remain fail
closed until their mode is changed. `IPV6_EGRESS_ENABLED` remains a legacy
bootstrap override only.

Run the local source-address integration check with:

```bash
./tests/ipv6-egress-docker.sh
./tests/he-ipv6-tunnel-docker.sh
./tests/ipv6-egress-sidecar-docker.sh
```

The sidecar test builds a real SIT tunnel between containers, keeps the main
application container free of network capabilities, and verifies two
free-bound account source addresses through the routed prefix.

> **Note:** The `docker-deploy.sh` script automatically generates `JWT_SECRET`, `TOTP_ENCRYPTION_KEY`, and `POSTGRES_PASSWORD` for you.

For load-balanced replicas, follow [MULTI_INSTANCE.md](./MULTI_INSTANCE.md).

### Easy Migration (Local Directory Version)

When using `docker-compose.local.yml`, all data is stored in local directories, making migration simple:

```bash
# On source server: Stop services and create archive
cd /path/to/deployment
docker compose -f docker-compose.local.yml down
cd ..
tar czf sub2api-complete.tar.gz deployment/

# Transfer to new server
scp sub2api-complete.tar.gz user@new-server:/path/to/destination/

# On new server: Extract and start
tar xzf sub2api-complete.tar.gz
cd deployment/
docker compose -f docker-compose.local.yml up -d
```

Your entire deployment (configuration + data) is migrated!

---

## Gemini OAuth Configuration

Sub2API supports three methods to connect to Gemini:

### Method 1: Code Assist OAuth (Recommended for GCP Users)

**No configuration needed** - always uses the built-in Gemini CLI OAuth client (public).

1. Leave `GEMINI_OAUTH_CLIENT_ID` and `GEMINI_OAUTH_CLIENT_SECRET` empty
2. In the Admin UI, create a Gemini OAuth account and select **"Code Assist"** type
3. Complete the OAuth flow in your browser

> Note: Even if you configure `GEMINI_OAUTH_CLIENT_ID` / `GEMINI_OAUTH_CLIENT_SECRET` for AI Studio OAuth,
> Code Assist OAuth will still use the built-in Gemini CLI client.

**Requirements:**
- Google account with access to Google Cloud Platform
- A GCP project (auto-detected or manually specified)

**How to get Project ID (if auto-detection fails):**
1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Click the project dropdown at the top of the page
3. Copy the Project ID (not the project name) from the list
4. Common formats: `my-project-123456` or `cloud-ai-companion-xxxxx`

### Method 2: AI Studio OAuth (For Regular Google Accounts)

Requires your own OAuth client credentials.

**Step 1: Create OAuth Client in Google Cloud Console**

1. Go to [Google Cloud Console - Credentials](https://console.cloud.google.com/apis/credentials)
2. Create a new project or select an existing one
3. **Enable the Generative Language API:**
   - Go to "APIs & Services" → "Library"
   - Search for "Generative Language API"
   - Click "Enable"
4. **Configure OAuth Consent Screen** (if not done):
   - Go to "APIs & Services" → "OAuth consent screen"
   - Choose "External" user type
   - Fill in app name, user support email, developer contact
   - Add scopes: `https://www.googleapis.com/auth/generative-language.retriever` (and optionally `https://www.googleapis.com/auth/cloud-platform`)
   - Add test users (your Google account email)
5. **Create OAuth 2.0 credentials:**
   - Go to "APIs & Services" → "Credentials"
   - Click "Create Credentials" → "OAuth client ID"
   - Application type: **Web application** (or **Desktop app**)
   - Name: e.g., "Sub2API Gemini"
   - Authorized redirect URIs: Add `http://localhost:1455/auth/callback`
6. Copy the **Client ID** and **Client Secret**
7. **⚠️ Publish to Production (IMPORTANT):**
   - Go to "APIs & Services" → "OAuth consent screen"
   - Click "PUBLISH APP" to move from Testing to Production
   - **Testing mode limitations:**
     - Only manually added test users can authenticate (max 100 users)
     - Refresh tokens expire after 7 days
     - Users must be re-added periodically
   - **Production mode:** Any Google user can authenticate, tokens don't expire
   - Note: For sensitive scopes, Google may require verification (demo video, privacy policy)

**Step 2: Configure Environment Variables**

```bash
GEMINI_OAUTH_CLIENT_ID=your-client-id.apps.googleusercontent.com
GEMINI_OAUTH_CLIENT_SECRET=GOCSPX-your-client-secret

# 可选：如需使用 Gemini CLI 内置 OAuth Client（Code Assist / Google One）
# 安全说明：本仓库不会内置该 client_secret，请在运行环境通过环境变量注入。
# GEMINI_CLI_OAUTH_CLIENT_SECRET=GOCSPX-your-built-in-secret
```

**Step 3: Create Account in Admin UI**

1. Create a Gemini OAuth account and select **"AI Studio"** type
2. Complete the OAuth flow
   - After consent, your browser will be redirected to `http://localhost:1455/auth/callback?code=...&state=...`
   - Copy the full callback URL (recommended) or just the `code` and paste it back into the Admin UI

### Method 3: API Key (Simplest)

1. Go to [Google AI Studio](https://aistudio.google.com/app/apikey)
2. Click "Create API key"
3. In Admin UI, create a Gemini **API Key** account
4. Paste your API key (starts with `AIza...`)

### Comparison Table

| Feature | Code Assist OAuth | AI Studio OAuth | API Key |
|---------|-------------------|-----------------|---------|
| Setup Complexity | Easy (no config) | Medium (OAuth client) | Easy |
| GCP Project Required | Yes | No | No |
| Custom OAuth Client | No (built-in) | Yes (required) | N/A |
| Rate Limits | GCP quota | Standard | Standard |
| Best For | GCP developers | Regular users needing OAuth | Quick testing |

---

## Binary Installation

For production servers using systemd.

### One-Line Installation

```bash
curl -fsSL https://raw.githubusercontent.com/DR-lin-eng/sub2api-no2api/main/deploy/install.sh -o sub2api-install.sh && chmod 700 sub2api-install.sh
# Review the downloaded installer before granting root access.
sudo bash ./sub2api-install.sh
```

### Manual Installation

1. Download the latest release from [GitHub Releases](https://github.com/DR-lin-eng/sub2api-no2api/releases)
2. Extract and copy the binary to `/opt/sub2api/`
3. Copy `sub2api.service` to `/etc/systemd/system/`
4. Run:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable sub2api
   sudo systemctl start sub2api
   ```
5. Open the Setup Wizard in your browser to complete configuration

### Commands

```bash
# Install
sudo ./install.sh

# Upgrade
sudo ./install.sh upgrade

# Uninstall
sudo ./install.sh uninstall
```

### Service Management

```bash
# Start the service
sudo systemctl start sub2api

# Stop the service
sudo systemctl stop sub2api

# Restart the service
sudo systemctl restart sub2api

# Check status
sudo systemctl status sub2api

# View logs
sudo journalctl -u sub2api -f

# Enable auto-start on boot
sudo systemctl enable sub2api
```

### Configuration

#### Server Address and Port

During installation, you will be prompted to configure the server listen address and port. These settings are stored in the systemd service file as environment variables.

To change after installation:

1. Edit the systemd service:
   ```bash
   sudo systemctl edit sub2api
   ```

2. Add or modify:
   ```ini
   [Service]
   Environment=SERVER_HOST=0.0.0.0
   Environment=SERVER_PORT=3000
   ```

3. Reload and restart:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl restart sub2api
   ```

#### Gemini OAuth Configuration

If you need to use AI Studio OAuth for Gemini accounts, add the OAuth client credentials to the systemd service file:

1. Edit the service file:
   ```bash
   sudo nano /etc/systemd/system/sub2api.service
   ```

2. Add your OAuth credentials in the `[Service]` section (after the existing `Environment=` lines):
   ```ini
   Environment=GEMINI_OAUTH_CLIENT_ID=your-client-id.apps.googleusercontent.com
   Environment=GEMINI_OAUTH_CLIENT_SECRET=GOCSPX-your-client-secret
   ```

   如需使用“内置 Gemini CLI OAuth Client”（Code Assist / Google One），还需要注入：
   ```ini
   Environment=GEMINI_CLI_OAUTH_CLIENT_SECRET=GOCSPX-your-built-in-secret
   ```

3. Reload and restart:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl restart sub2api
   ```

> **Note:** Code Assist OAuth does not require any configuration - it uses the built-in Gemini CLI client.
> See the [Gemini OAuth Configuration](#gemini-oauth-configuration) section above for detailed setup instructions.

#### Application Configuration

The main config file is at `/etc/sub2api/config.yaml` (created by Setup Wizard).

### Prerequisites

- Linux server (Ubuntu 20.04+, Debian 11+, CentOS 8+, etc.)
- PostgreSQL 14+
- Redis 6+
- systemd

### Directory Structure

```
/opt/sub2api/
├── sub2api              # Main binary
├── sub2api.backup       # Backup (after upgrade)
└── data/                # Runtime data

/etc/sub2api/
└── config.yaml          # Configuration file
```

---

## Troubleshooting

### Docker

For **local directory version**:

```bash
# Check container status
docker compose -f docker-compose.local.yml ps

# View detailed logs
docker compose -f docker-compose.local.yml logs --tail=100 sub2api

# Check database connection
docker compose -f docker-compose.local.yml exec postgres pg_isready

# Check Redis connection
docker compose -f docker-compose.local.yml exec redis redis-cli ping

# Restart all services
docker compose -f docker-compose.local.yml restart

# Check data directories
ls -la data/ postgres_data/
```

For **named volumes version**:

```bash
# Check container status
docker compose ps

# View detailed logs
docker compose logs --tail=100 sub2api

# Check database connection
docker compose exec postgres pg_isready

# Check Redis connection
docker compose exec redis redis-cli ping

# Restart all services
docker compose restart
```

### Binary Install

```bash
# Check service status
sudo systemctl status sub2api

# View recent logs
sudo journalctl -u sub2api -n 50

# Check config file
sudo cat /etc/sub2api/config.yaml

# Check PostgreSQL
sudo systemctl status postgresql

# Check Redis
sudo systemctl status redis
```

### Common Issues

1. **Port already in use**: Change `SERVER_PORT` in `.env` or systemd config
2. **Database connection failed**: Check PostgreSQL is running and credentials are correct
3. **Redis connection failed**: Check Redis is running and password is correct
4. **Permission denied**: Ensure proper file ownership for binary install

---

## TLS Fingerprint Configuration

Sub2API supports TLS fingerprint simulation to make requests appear as if they come from the official Claude CLI (Node.js client).

> **💡 Tip:** Visit **[tls.sub2api.org](https://tls.sub2api.org/)** to get TLS fingerprint information for different devices and browsers.

### Default Behavior

- Built-in `claude_cli_v2` profile simulates Node.js 20.x + OpenSSL 3.x
- JA3 Hash: `1a28e69016765d92e3b381168d68922c`
- JA4: `t13d5911h1_a33745022dd6_1f22a2ca17c4`
- Profile selection: `accountID % profileCount`

### Configuration

```yaml
gateway:
  tls_fingerprint:
    enabled: true  # Global switch
    profiles:
      # Simple profile (uses default cipher suites)
      profile_1:
        name: "Profile 1"

      # Profile with custom cipher suites (use compact array format)
      profile_2:
        name: "Profile 2"
        cipher_suites: [4866, 4867, 4865, 49199, 49195, 49200, 49196]
        curves: [29, 23, 24]
        point_formats: 0

      # Another custom profile
      profile_3:
        name: "Profile 3"
        cipher_suites: [4865, 4866, 4867, 49199, 49200]
        curves: [29, 23, 24, 25]
```

### Profile Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name (required) |
| `cipher_suites` | []uint16 | Cipher suites in decimal. Empty = default |
| `curves` | []uint16 | Elliptic curves in decimal. Empty = default |
| `point_formats` | []uint8 | EC point formats. Empty = default |

### Common Values Reference

**Cipher Suites (TLS 1.3):** `4865` (AES_128_GCM), `4866` (AES_256_GCM), `4867` (CHACHA20)

**Cipher Suites (TLS 1.2):** `49195`, `49196`, `49199`, `49200` (ECDHE variants)

**Curves:** `29` (X25519), `23` (P-256), `24` (P-384), `25` (P-521)
