package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanSchemaTupleItemsAndPropertiesAreBothVisited(t *testing.T) {
	for _, rawItems := range []any{nil, false, true, map[string]any{}} {
		schema := map[string]any{"type": "object", "properties": map[string]any{
			"values": map[string]any{"type": "array", "prefixItems": []any{map[string]any{"type": "string"}}, "items": rawItems},
		}}
		cleaned := CleanJSONSchema(schema)
		properties, ok := cleaned["properties"].(map[string]any)
		require.True(t, ok)
		array, ok := properties["values"].(map[string]any)
		require.True(t, ok)
		require.NotContains(t, array, "prefixItems")
		items, ok := array["items"].(map[string]any)
		require.True(t, ok)
		require.NotEmpty(t, items["type"])
	}
}

func TestCleanSchemaRetainsDeclaredArrayItems(t *testing.T) {
	cleaned := CleanJSONSchema(map[string]any{"type": "array", "prefixItems": []any{map[string]any{"type": "string"}}, "items": map[string]any{"type": "integer"}})
	items, ok := cleaned["items"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "integer", items["type"])
}

func TestCleanSchemaEnumOnlyInfersScalar(t *testing.T) {
	cleaned := CleanJSONSchema(map[string]any{"enum": []any{"one", "two"}})
	require.Equal(t, "string", cleaned["type"])
	require.NotContains(t, cleaned, "properties")
}
