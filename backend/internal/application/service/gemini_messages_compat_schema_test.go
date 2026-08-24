package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanToolSchema_RemovesNestedDeprecatedAndNormalizesMixedScalarEnum(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{
			map[string]any{"type": "string", "deprecated": true},
			map[string]any{"enum": []any{"enabled", false, float64(1), nil}},
		},
	}

	cleaned, ok := cleanToolSchema(schema).(map[string]any)
	require.True(t, ok)
	anyOf, ok := cleaned["anyOf"].([]any)
	require.True(t, ok)
	deprecatedSchema, ok := anyOf[0].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, deprecatedSchema, "deprecated")
	enumSchema, ok := anyOf[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"enabled", "false", "1", "null"}, enumSchema["enum"])
}

func TestCleanToolSchema_DropsEnumWithNonScalarValue(t *testing.T) {
	cleaned, ok := cleanToolSchema(map[string]any{
		"enum": []any{"valid", map[string]any{"invalid": true}},
	}).(map[string]any)
	require.True(t, ok)
	require.NotContains(t, cleaned, "enum")
}
