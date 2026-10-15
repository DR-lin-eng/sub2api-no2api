package claude

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestClaude55ProviderAndTierIDs(t *testing.T) {
	for _, family := range []string{"opus", "sonnet", "haiku"} {
		for _, id := range []string{"claude-" + family + "-5-5", "anthropic/claude-" + family + "-5.5", "us.anthropic.claude-" + family + "-5-5-20261007-v1:0", "claude-" + family + "-5-5-high"} {
			fn := IsOpus55
			if family == "sonnet" {
				fn = IsSonnet55
			}
			if family == "haiku" {
				fn = IsHaiku55
			}
			require.True(t, fn(id), id)
			require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, EffortLevelsForModel(id))
		}
	}
	require.False(t, IsOpus55("claude-opus-5-50"))
	require.False(t, IsOpus55("claude-opus-5"))
}
