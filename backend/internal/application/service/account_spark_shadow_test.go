package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountSparkShadowHelpers(t *testing.T) {
	pid := int64(100)
	normal := &Account{ID: 100}
	require.False(t, normal.IsShadow())
	require.False(t, normal.IsCredentialShadow())
	require.Equal(t, QuotaDimensionGlobal, normal.QuotaDimensionOrDefault())
	shadow := &Account{ID: 200, ParentAccountID: &pid, QuotaDimension: QuotaDimensionSpark}
	require.True(t, shadow.IsShadow())
	require.True(t, shadow.IsCredentialShadow())
	require.Equal(t, QuotaDimensionSpark, shadow.QuotaDimensionOrDefault())
}

func TestCodexVirtualClientKeyPrefersPrincipalAndShadowNamespace(t *testing.T) {
	parent := &Account{
		ID:       100,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "acct-shared",
		},
	}
	require.Equal(t, "chatgpt:acct-shared", parent.CodexVirtualClientKey())
	pid := parent.ID
	shadow := &Account{ID: 200, ParentAccountID: &pid, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Equal(t, "parent:100", shadow.CodexVirtualClientKey())
	shadow.Extra = map[string]any{CodexVirtualClientKeyExtraKey: parent.CodexVirtualClientKey()}
	require.Equal(t, parent.CodexVirtualClientKey(), shadow.CodexVirtualClientKey())
}
