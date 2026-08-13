//go:build unit

package xai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModelMappingIncludesGrok46(t *testing.T) {
	mapping := DefaultModelMapping()
	require.Equal(t, "grok-4.6", mapping["grok-4.6"])
	require.Equal(t, "grok-4.6", mapping["grok-4.6-latest"])
	require.Contains(t, DefaultModelIDs(), "grok-4.6")
}

func TestStripGrokProviderPrefix(t *testing.T) {
	require.Equal(t, "grok-4.6", StripGrokProviderPrefix(" x-ai/grok-4.6 "))
	require.Equal(t, "vendor/model", StripGrokProviderPrefix("vendor/model"))
}

func TestResolveGrokTextResponsesModelID(t *testing.T) {
	tests := map[string]string{
		"":                                   DefaultTextModel,
		"grok-latest":                        DefaultTextModel,
		" xai/grok-4.6-latest ":              "grok-4.6",
		"grok-4.3-latest":                    "grok-4.3",
		"grok-build":                         "grok-build-0.1",
		"grok-4.20-multi-agent-latest":       "grok-4.20-multi-agent-0309",
		"x-ai/grok-private-preview-model-id": "grok-private-preview-model-id",
	}
	for input, want := range tests {
		require.Equal(t, want, ResolveGrokTextResponsesModelID(input), input)
	}
	require.Equal(t, "operator-default", ResolveGrokTextResponsesModelID("grok", "operator-default"))
}
