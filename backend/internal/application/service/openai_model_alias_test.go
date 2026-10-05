package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeKnownOpenAICodexModel_BareGPT56RoutesToSol(t *testing.T) {
	tests := map[string]string{
		"gpt-5.6":            "gpt-5.6-sol",
		"openai/gpt-5.6":     "gpt-5.6-sol",
		"gpt5.6":             "gpt-5.6-sol",
		"gpt-5.6-high":       "gpt-5.6-sol",
		"gpt-5.6-max":        "gpt-5.6-sol",
		"gpt-5.6-2026-07-09": "gpt-5.6-sol",
		"openai/gpt-5.6-max": "gpt-5.6-sol",
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeKnownOpenAICodexModel(input))
		})
	}
}

func TestUsageBillingModelCandidates_BareGPT56IncludesSol(t *testing.T) {
	require.Equal(t,
		[]string{"gpt-5.6", "gpt-5.6-sol"},
		usageBillingModelCandidates("gpt-5.6"),
	)
	require.Equal(t,
		[]string{"openai/gpt-5.6", "gpt-5.6", "gpt-5.6-sol"},
		usageBillingModelCandidates("openai/gpt-5.6"),
	)
}

func TestNormalizeKnownOpenAICodexModel_GPT6AstraAliases(t *testing.T) {
	for input, expected := range map[string]string{
		"gpt-6-astra":                "gpt-6-astra",
		"openai/gpt-6-astra":         "gpt-6-astra",
		"gpt6_astra":                 "gpt-6-astra",
		"gpt-6-astra-max":            "gpt-6-astra",
		"gpt-6-astra-2026-09-04":     "gpt-6-astra",
		"gpt-6-astra-openai-compact": "gpt-6-astra",
	} {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeKnownOpenAICodexModel(input))
		})
	}
	require.Empty(t, normalizeKnownOpenAICodexModel("gpt-6"))
	require.Empty(t, normalizeKnownOpenAICodexModel("gpt-6-orion"))
}

func TestNormalizeKnownOpenAICodexModel_GPT61SolAliases(t *testing.T) {
	for input, expected := range map[string]string{
		"gpt-6.1-sol":                "gpt-6.1-sol",
		"openai/gpt6.1_sol":          "gpt-6.1-sol",
		"gpt-6.1-sol-max":            "gpt-6.1-sol",
		"gpt-6.1-sol-2026-09-29":     "gpt-6.1-sol",
		"gpt-6.1-sol-openai-compact": "gpt-6.1-sol",
	} {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, normalizeKnownOpenAICodexModel(input))
		})
	}
	require.True(t, isOpenAIGPT61SolModel("gpt-6.1-sol"))
	require.True(t, isOpenAIGPT6Model("gpt-6.1-sol"))
	require.Equal(t, "low", normalizeOpenAIReasoningEffortForModel("none", "gpt-6.1-sol"))
	require.Equal(t, "low", normalizeOpenAIReasoningEffortForModel("minimal", "gpt-6.1-sol"))
	value, changed := normalizeOpenAIReasoningEffortForUpstream("minimal", "gpt-6.1-sol")
	require.True(t, changed)
	require.Equal(t, "low", value)
}
