# Management MCP

This feature owns OAuth consent and authorized MCP connections. It is hidden until the server explicitly advertises `mcp_enabled: true`; a missing flag from an old server keeps it off.

- `data/datasources/mcpDatasource.ts` uses the shared HTTP client for connection configuration, consent and grant revocation.
- `presentation/pages/MCPOAuthAuthorizePage.vue` previews the exact immutable request, defaults to personal/read-only authority, requires explicit approval and uses the existing TOTP step-up controller for administrator consent. Route changes and unmount invalidate pending actions.
- `presentation/pages/MCPConnectionsPage.vue` displays the configured resource and bounded grant history, with explicit revocation confirmation.
- `__tests__/` covers malformed/repeated query parameters, untrusted application labels, mismatched redirects, permission choices, step-up cancellation and late responses.

Browser JWTs remain memory-only; this feature never receives or persists MCP access/refresh tokens. Localized text belongs to `core/i18n/locales/{en,zh}/mcp.ts` and loads in the existing user scope for both MCP routes.
