package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCleanToolSchema_PreservesSchemaKeywordNamedProperties(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"examples": []any{"root example"},
		"properties": map[string]any{
			"const": map[string]any{
				"type":  "string",
				"const": "property const",
			},
			"enum": map[string]any{
				"type":  "string",
				"const": "property enum",
			},
			"examples": map[string]any{
				"type": "string",
			},
			"type": map[string]any{
				"type": "string",
			},
		},
	}

	cleaned, ok := cleanToolSchema(schema).(map[string]any)
	require.True(t, ok)
	require.NotContains(t, cleaned, "examples")

	properties, ok := cleaned["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, properties, "const")
	require.Contains(t, properties, "enum")
	require.Contains(t, properties, "examples")
	require.Contains(t, properties, "type")

	constSchema, ok := properties["const"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"property const"}, constSchema["enum"])

	enumPropertySchema, ok := properties["enum"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"property enum"}, enumPropertySchema["enum"])
}

func TestCleanToolSchema_NormalizesConstAndExamplesAtNestedSchemaNodes(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"values": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "string",
					"examples": []any{"item example"},
					"anyOf": []any{
						map[string]any{"type": "string", "const": "a"},
						map[string]any{"type": "string", "examples": []any{"branch example"}},
					},
				},
			},
		},
	}

	cleaned, ok := cleanToolSchema(schema).(map[string]any)
	require.True(t, ok)
	properties, ok := cleaned["properties"].(map[string]any)
	require.True(t, ok)
	values, ok := properties["values"].(map[string]any)
	require.True(t, ok)
	items, ok := values["items"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, items, "examples")
	anyOf, ok := items["anyOf"].([]any)
	require.True(t, ok)
	require.Len(t, anyOf, 2)
	firstBranch, ok := anyOf[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"a"}, firstBranch["enum"])
	secondBranch, ok := anyOf[1].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, secondBranch, "examples")
}

func TestCleanToolSchema_TraversesRetainedSchemaValuedKeywords(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"additionalItems": map[string]any{
			"type":     "string",
			"const":    "additional",
			"examples": []any{"additional example"},
		},
		"unevaluatedItems": map[string]any{
			"type":     "string",
			"const":    "unevaluated item",
			"examples": []any{"item example"},
		},
		"unevaluatedProperties": map[string]any{
			"type":     "string",
			"const":    "unevaluated property",
			"examples": []any{"property example"},
		},
		"dependencies": map[string]any{
			"schema dependency": map[string]any{
				"type":     "string",
				"const":    "dependency",
				"examples": []any{"dependency example"},
			},
			"required dependency": []any{"first", "second"},
		},
	}

	cleaned, ok := cleanToolSchema(schema).(map[string]any)
	require.True(t, ok)
	expectedEnums := map[string][]any{
		"additionalItems":       {"additional"},
		"unevaluatedItems":      {"unevaluated item"},
		"unevaluatedProperties": {"unevaluated property"},
	}
	for keyword, expectedEnum := range expectedEnums {
		child, ok := cleaned[keyword].(map[string]any)
		require.True(t, ok)
		require.Equal(t, expectedEnum, child["enum"])
		require.NotContains(t, child, "const")
		require.NotContains(t, child, "examples")
	}

	dependencies, ok := cleaned["dependencies"].(map[string]any)
	require.True(t, ok)
	dependencySchema, ok := dependencies["schema dependency"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"dependency"}, dependencySchema["enum"])
	require.NotContains(t, dependencySchema, "const")
	require.NotContains(t, dependencySchema, "examples")
	require.Equal(t, []any{"first", "second"}, dependencies["required dependency"])
}

func TestCleanToolSchema_IntersectsStringConstWithEnum(t *testing.T) {
	tests := []struct {
		name       string
		constValue string
		enum       []any
		expected   []any
	}{
		{name: "matching", constValue: "browser", enum: []any{"browser", "shell"}, expected: []any{"browser"}},
		{name: "contradictory", constValue: "browser", enum: []any{"shell"}, expected: []any{}},
		{name: "numeric string collision", constValue: "7", enum: []any{float64(7)}, expected: []any{}},
		{name: "empty string", constValue: "", enum: []any{"", "shell"}, expected: []any{""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := map[string]any{
				"type":  "string",
				"const": test.constValue,
				"enum":  test.enum,
			}

			cleaned, ok := cleanToolSchema(schema).(map[string]any)
			require.True(t, ok)
			require.NotContains(t, cleaned, "const")
			require.Equal(t, test.expected, cleaned["enum"])
		})
	}

	cleaned, ok := cleanToolSchema(map[string]any{"const": "browser"}).(map[string]any)
	require.True(t, ok)
	require.Equal(t, "STRING", cleaned["type"])
	require.Equal(t, []any{"browser"}, cleaned["enum"])
}

func TestCleanToolSchema_DropsNonStringConstWithoutInferringType(t *testing.T) {
	for _, constValue := range []any{true, float64(7), map[string]any{"value": "x"}} {
		cleaned, ok := cleanToolSchema(map[string]any{"const": constValue}).(map[string]any)
		require.True(t, ok)
		require.NotContains(t, cleaned, "const")
		require.NotContains(t, cleaned, "type")
		require.NotContains(t, cleaned, "enum")
	}
}

func TestCleanToolSchema_DropsNonStringConstAndRetainsExistingTypeAndEnum(t *testing.T) {
	schema := map[string]any{
		"type":  "integer",
		"const": float64(7),
		"enum":  []any{float64(8)},
	}

	cleaned, ok := cleanToolSchema(schema).(map[string]any)
	require.True(t, ok)
	require.NotContains(t, cleaned, "const")
	require.Equal(t, "INTEGER", cleaned["type"])
	require.Equal(t, []any{"8"}, cleaned["enum"])
}

func TestCleanToolSchema_PreservesNestedOneOfCombinator(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{
				"oneOf": []any{
					map[string]any{"const": "a"},
					map[string]any{"type": "string", "const": "b"},
				},
			},
		},
	}

	cleaned, ok := cleanToolSchema(schema).(map[string]any)
	require.True(t, ok)
	properties, ok := cleaned["properties"].(map[string]any)
	require.True(t, ok)
	valueSchema, ok := properties["value"].(map[string]any)
	require.True(t, ok)
	oneOf, ok := valueSchema["oneOf"].([]any)
	require.True(t, ok)
	require.Len(t, oneOf, 2)
	firstBranch, ok := oneOf[0].(map[string]any)
	require.True(t, ok)
	secondBranch, ok := oneOf[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []any{"a"}, firstBranch["enum"])
	require.Equal(t, []any{"b"}, secondBranch["enum"])
}
