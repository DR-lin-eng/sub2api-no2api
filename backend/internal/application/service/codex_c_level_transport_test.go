package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/stretchr/testify/require"
)

func TestCLevelGateControlsVirtualClientTransportKey(t *testing.T) {
	account := &Account{
		ID:       42,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "principal-c",
		},
	}
	codexsimulation.SetCLevelEnabled(false)
	t.Cleanup(func() { codexsimulation.SetCLevelEnabled(false) })
	require.Empty(t, openAIVirtualClientKey(account))

	codexsimulation.SetCLevelEnabled(true)
	require.Equal(t, "chatgpt:principal-c", openAIVirtualClientKey(account))
}
