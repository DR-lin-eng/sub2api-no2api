package managementmcp

import policy "github.com/DR-lin-eng/sub2api-no2api/internal/modules/managementmcp"

// ToolSpec and ToolCatalog expose the immutable domain catalog to HTTP adapters.
type ToolSpec = policy.ToolSpec

func ToolCatalog() []ToolSpec { return policy.ToolCatalog() }
