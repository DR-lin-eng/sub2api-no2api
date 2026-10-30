# Management MCP #8389 compatibility verification

Base: `8119c146220aae5ca21abae54c14299aac0b04d9`  
Candidate: `dcccdd7bd00ad9d51e82800c6ff52dcbd6525536`  
Upstream source: #8389 merge `7386bc6ae0659f192199067fc5060d2ad33622fe`

The implementation uses the current module/application/infrastructure/transport owners. It keeps the existing root OAuth2 discovery endpoint unchanged, uses MCP-specific discovery under `/.well-known/oauth-authorization-server/api/v1/mcp`, and remains disabled until `MCP_PUBLIC_URL` is configured. Database migration `252_mcp_oauth.sql` is forward-only and does not modify the already published migration 249.

## Checks

| Check | Result |
| --- | --- |
| Frontend MCP pages and consent tests | Passed; 31 tests |
| Frontend route, locale, step-up and MCP owner tests | Passed; 21 files / 155 tests |
| Frontend typecheck and ESLint | Passed |
| Docker Go 1.26.10 targeted MCP tests | Passed |
| Docker Go 1.26.10 full `go test -tags=unit ./... -count=1` | Passed |
| Official Go MCP SDK OAuth interoperability | Passed; discovery, dynamic registration, S256 PKCE, consent, tools/list, tools/call and refresh |
| Docker Testcontainers PostgreSQL/Redis integration | Passed; concurrent exchange, replay revocation, grant limit, persistent client key, old-node status/email/password mutation revocation, and token authority |
| Joined authentication query benchmark | Passed; 100 calls per run, 3 runs. Joined query: 188.1ms / 595.4ms / 252.9ms; previous four-query shape: 591.9ms / 1.131s / 850.8ms |
| Production Docker image | Passed; `sub2api:management-mcp-8389`, Go embed and runtime startup build |
| Production runtime discovery fixture | Passed; isolated PostgreSQL/Redis/app returned MCP protected-resource metadata and MCP-specific issuer/token endpoints. Existing root OAuth2 path remained a separate route (fixture had OAuth2 disabled and returned its existing disabled response). Containers and volumes were removed after the check. |
| `make check-docs` and `git diff --check` | Passed |

The first frontend owner run failed only because the new `mcp` locale was not included in the existing route-locale test fixture; the fixture was updated and the final run passed. No test was skipped to obtain the final result.

## Security review

Codex Security diff scan `12017249-daa8-4315-bc63-673dc31989f6` completed with complete coverage of the 81 changed review items and zero reportable findings. The review specifically covered OAuth/PKCE binding and replay, live role/TOTP/session authority, private fixed-schema dispatch, redaction, resource/rate limits, SQL races, old-node mutation triggers, frontend consent and upgrade/rollback boundaries.

The only retained deployment assumption is that a reverse proxy normalizes the request `Host` header before the same-host Origin shortcut is used. MCP never authenticates with cookies and always requires a resource-bound bearer token; no repository-reachable bearer-token bypass was found.

## Upgrade and rollback

MCP is enabled only after all backend nodes support migration 252 and the stable public resource URL is configured. Old frontend nodes hide the feature when `mcp_enabled` is absent. Before binary rollback, clear `MCP_PUBLIC_URL` and stop routing MCP requests to old nodes; leave migration 252 and its revocation trigger in place. Old-node writes that disable, demote, delete, change password or change email revoke grants through the database trigger, so restoring the account does not revive prior authorization.
