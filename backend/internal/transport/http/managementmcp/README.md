# Management MCP HTTP adapter

`auth.go` validates resource-bound OAuth access tokens and installs a non-forgeable private dispatch context. `server.go` adapts the fixed catalog in `modules/managementmcp` to the MCP Go SDK, using stateless Streamable HTTP, closed schemas and bounded request/result buffers. `catalog.go` exposes the domain catalog without duplicating policy.

Only the private in-process router accepts this dispatch context; public REST authentication remains unchanged. Every HTTP request rereads grant and user authority. Tools cannot call arbitrary URLs, change identity or obtain existing credentials. Account extra/proxy data and secrets are removed from results; only a newly created user key may be returned once.

See [management MCP deployment and upgrade guide](../../../../../docs/MANAGEMENT_MCP.md).
