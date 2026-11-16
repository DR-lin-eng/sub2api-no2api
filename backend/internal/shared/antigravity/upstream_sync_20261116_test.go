package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncSchemaItemsFollowsFinalSelectedType(t *testing.T) {
	for _, typ := range []any{"object", "string", []any{"string", "array"}} {
		schema := map[string]any{"type": typ, "items": map[string]any{"type": "string"}}
		out := CleanJSONSchema(schema)
		require.NotContains(t, out, "items")
	}
	props := CleanJSONSchema(map[string]any{"type": "object", "properties": map[string]any{"items": map[string]any{"type": "string"}}})
	properties, ok := props["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, properties, "items")
	out := CleanJSONSchema(map[string]any{"type": "array", "items": []any{map[string]any{"type": "integer"}}})
	items, ok := out["items"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "integer", items["type"], "keep the existing tuple adapter instead of degrading its first item")
}
