# Management MCP #8389 compatibility verification

Base: `8119c146220aae5ca21abae54c14299aac0b04d9`
Candidate: `8996f45cb509f29a39c2e6bd30fc824e1e09a360`
Upstream source: #8389 merge `7386bc6ae0659f192199067fc5060d2ad33622fe`

The implementation uses the current module/application/infrastructure/transport owners. It keeps the existing root OAuth2 discovery endpoint unchanged, uses MCP-specific discovery under `/.well-known/oauth-authorization-server/api/v1/mcp`, and remains disabled until `MCP_PUBLIC_URL` is configured. Database migration `252_mcp_oauth.sql` is forward-only and does not modify the already published migration 249.

## Checks

| Check | Result |
| --- | --- |
| Frontend MCP pages and consent tests | Passed; 31 tests |
| Frontend route, locale, step-up and MCP owner tests | Passed; 21 files / 155 tests |
| Frontend typecheck and ESLint | Passed |
| Final frontend critical suite | Passed; 31 files / 279 tests |
| Docker Go 1.26.10 targeted MCP tests | Passed |
| Docker Go 1.26.10 full `go test -tags=unit ./... -count=1` | Passed |
| Official Go MCP SDK OAuth interoperability | Passed; discovery, dynamic registration, S256 PKCE, consent, tools/list, tools/call and refresh |
| Docker Testcontainers PostgreSQL/Redis integration | Passed; concurrent exchange, replay revocation, grant limit, persistent client key, old-node status/email/password mutation revocation, and token authority |
| Joined authentication query benchmark | Passed; 100 calls per run, 3 runs. Joined query: 188.1ms / 595.4ms / 252.9ms; previous four-query shape: 591.9ms / 1.131s / 850.8ms |
| Production Docker image | Passed; `sub2api:management-mcp-8389`, Go embed and runtime startup build |
| Production runtime discovery fixture | Passed; isolated PostgreSQL/Redis/app returned MCP protected-resource metadata and MCP-specific issuer/token endpoints. Existing root OAuth2 path remained a separate route (fixture had OAuth2 disabled and returned its existing disabled response). Containers and volumes were removed after the check. |
| `make check-docs` and `git diff --check` | Passed |
| Same-database `BASELINE → MODIFIED → ROLLBACK → RESTORED` | Passed; migration count `307 → 308 → 308 → 308`; identity/key/subscription snapshot hash stayed `7eef93b58e9d6d34b90ad49963e59559d0a4bd13c75f4088773d778278f4775a`; old-node user writes revoked grants and restored build issued a fresh grant |
| Browser UI fixture | Passed; MCP connections page at 1488px desktop and 390×844 mobile had no horizontal overflow; resource URL, grant rows and read/write/read-only labels rendered |

The first frontend owner run failed only because the new `mcp` locale was not included in the existing route-locale test fixture; the fixture was updated and the final run passed. An early SDK test invocation also needed its existing build tag restored, and the final Make target now runs that official interoperability test explicitly. No test was skipped to obtain the final result. OAuth authorization requests now require a client-generated `state`, which is returned unchanged and documented as mandatory for callback CSRF protection.

## Security review

Codex Security diff scan `0e421ad1-aef9-4e01-816c-88a95bd3d6d9` completed with complete coverage of the 86 changed review items and zero reportable findings. The final redaction review also removes free-form account error fields so provider error bodies cannot cross the MCP boundary. The prior scan `12017249-daa8-4315-bc63-673dc31989f6` covered the initial implementation; the final scan covers the authorization-boundary fixes and mandatory OAuth state. The review specifically covered OAuth/PKCE/state binding and replay, live role/TOTP/session authority, private fixed-schema dispatch, redaction, resource/rate limits, SQL races, SQL-trigger old-node mutation revocation, frontend consent, backend-mode routing and upgrade/rollback boundaries.

The only retained deployment assumption is that a reverse proxy normalizes the request `Host` header before the same-host Origin shortcut is used. MCP never authenticates with cookies and always requires a resource-bound bearer token; no repository-reachable bearer-token bypass was found.

## Upgrade and rollback

MCP is enabled only after all backend nodes support migration 252 and the stable public resource URL is configured. Old frontend nodes hide the feature when `mcp_enabled` is absent. Before binary rollback, clear `MCP_PUBLIC_URL` and stop routing MCP requests to old nodes; leave migration 252 and its revocation trigger in place. Old-node writes that disable, demote, delete, change password or change email revoke grants through the database trigger, so restoring the account does not revive prior authorization.
