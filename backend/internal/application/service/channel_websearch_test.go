package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannel_IsWebSearchEmulationEnabled_Enabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": true},
		},
	}
	require.True(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_DifferentPlatform(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": true},
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("openai"))
}

func TestChannel_IsWebSearchEmulationEnabled_Disabled(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": false},
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_NilFeaturesConfig(t *testing.T) {
	c := &Channel{FeaturesConfig: nil}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_NilChannel(t *testing.T) {
	var c *Channel
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_WrongStructure(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: true, // not a map
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestChannel_IsWebSearchEmulationEnabled_PlatformValueNotBool(t *testing.T) {
	c := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{"anthropic": "yes"},
		},
	}
	require.False(t, c.IsWebSearchEmulationEnabled("anthropic"))
}

func TestMergeWebSearchEmulationDimensions_InheritsMissingPlatforms(t *testing.T) {
	existing := map[string]any{
		featureKeyWebSearchEmulation: map[string]any{"anthropic": true, "openai": true},
		"other":                      map[string]any{"enabled": true},
	}
	incoming := map[string]any{featureKeyWebSearchEmulation: map[string]any{"anthropic": false}}

	merged := mergeWebSearchEmulationDimensions(existing, incoming)
	dimensions, ok := merged[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, dimensions["anthropic"])
	require.Equal(t, true, dimensions["openai"])
	require.NotContains(t, merged, "other", "unrelated feature keys keep whole-block replacement semantics")
}

func TestMergeWebSearchEmulationDimensions_MissingKeyAndExplicitClear(t *testing.T) {
	existing := map[string]any{featureKeyWebSearchEmulation: map[string]any{"openai": true}}

	merged := mergeWebSearchEmulationDimensions(existing, map[string]any{"other": 1})
	dimensions, ok := merged[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, dimensions["openai"])
	dimensions["openai"] = false
	existingDimensions, ok := existing[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, existingDimensions["openai"], "inherited dimensions must be copied")

	cleared := mergeWebSearchEmulationDimensions(existing, map[string]any{
		featureKeyWebSearchEmulation: map[string]any{},
	})
	clearedDimensions, ok := cleared[featureKeyWebSearchEmulation].(map[string]any)
	require.True(t, ok)
	require.Empty(t, clearedDimensions)
}
