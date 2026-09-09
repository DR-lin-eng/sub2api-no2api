package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeGroupModelAllowlist(t *testing.T) {
	got, err := normalizeGroupModelAllowlist(GroupModelAllowlist{
		Enabled: true,
		Models:  []string{" gpt-5.4 ", "GPT-5.4", "gpt-5-*", ""},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5.4", "gpt-5-*"}, got.Models)

	_, err = normalizeGroupModelAllowlist(GroupModelAllowlist{Enabled: true})
	require.Error(t, err)
	_, err = normalizeGroupModelAllowlist(GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*x"}})
	require.Error(t, err)
}

func TestGroupModelAllowlistAllowsAliasesAndWildcards(t *testing.T) {
	allowlist := GroupModelAllowlist{Enabled: true, Models: []string{"claude-sonnet-4-6", "gpt-5-*", "gemini-2.5-*"}}
	for _, model := range []string{"claude-sonnet-4-6-thinking", "gpt-5-4", "models/gemini-2.5-pro"} {
		require.True(t, allowlist.Allows(model), model)
	}
	require.False(t, allowlist.Allows("gpt-4.1"))
}

func TestGroupModelAllowlistFilterForListing(t *testing.T) {
	allowlist := GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5-mini", "gpt-5-*"}}
	got := allowlist.FilterForListing([]string{"gpt-5-4", "gpt-5-mini", "gpt-4.1"})
	require.Equal(t, []string{"gpt-5-mini", "gpt-5-4"}, got)
}
