# Management MCP policy

This module owns the fixed management tool catalog, closed JSON schemas and mutability metadata. It does not access HTTP, SQL, credentials or application services.

OAuth orchestration and ports live in `application/service/mcp_oauth*`; SQL implementations live in `infrastructure/repository/mcp_oauth*`; Streamable HTTP/SDK dispatch lives in `transport/http/managementmcp/`. Public routing is in `transport/http/server/routes/mcp.go`.

Tests cover every schema, route binding and denied credential-management field. New tools must bind an existing management use case and keep its ownership, rate limits, audit and step-up checks.
