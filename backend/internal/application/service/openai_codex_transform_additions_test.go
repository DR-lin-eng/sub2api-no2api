package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/stretchr/testify/require"
)

// ensureCodexReasoningInclude：带 reasoning 时补齐 include，幂等且保留既有项。
func TestEnsureCodexReasoningInclude(t *testing.T) {
	// reasoning 存在、include 缺失 → 注入
	body := map[string]any{"reasoning": map[string]any{"effort": "medium"}}
	require.True(t, ensureCodexReasoningInclude(body))
	require.Equal(t, []any{"reasoning.encrypted_content"}, body["include"])
	// 幂等：再次调用不重复
	require.False(t, ensureCodexReasoningInclude(body))

	// 无 reasoning → 不动
	body2 := map[string]any{}
	require.False(t, ensureCodexReasoningInclude(body2))
	_, ok := body2["include"]
	require.False(t, ok)

	// 既有 include 保留并追加
	body3 := map[string]any{
		"reasoning": map[string]any{"effort": "high"},
		"include":   []any{"foo"},
	}
	require.True(t, ensureCodexReasoningInclude(body3))
	require.Equal(t, []any{"foo", "reasoning.encrypted_content"}, body3["include"])
}

// applyCodexClientMetadata：用账号稳定 installation/device 值覆盖客户端伪造值。
func TestApplyCodexClientMetadata(t *testing.T) {
	// 仅 OpenAI OAuth 账号才有 device_id（GetOpenAIDeviceID 的门控）。
	acc := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_device_id": "dev-xyz"}}

	body := map[string]any{}
	require.True(t, applyCodexClientMetadata(body, acc))
	cm, ok := body["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dev-xyz", cm["x-codex-installation-id"])
	// 幂等
	require.False(t, applyCodexClientMetadata(body, acc))

	// 客户端伪造值必须被账号真实值覆盖。
	spoofed := map[string]any{"client_metadata": map[string]any{"x-codex-installation-id": "forged-device"}}
	require.True(t, applyCodexClientMetadata(spoofed, acc))
	spoofedMetadata, ok := spoofed["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "dev-xyz", spoofedMetadata["x-codex-installation-id"])

	// OAuth 账号没有持久化 device_id 时，也必须使用稳定的账号派生值；已有伪造值会被覆盖。
	body2 := map[string]any{"client_metadata": map[string]any{"x-codex-installation-id": "forged-device"}}
	accountWithoutDevice := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, applyCodexClientMetadata(body2, accountWithoutDevice))
	body2Metadata, ok := body2["client_metadata"].(map[string]any)
	require.True(t, ok)
	stableDevice, ok := body2Metadata["x-codex-installation-id"].(string)
	require.True(t, ok)
	require.NotEqual(t, "forged-device", stableDevice)
	require.Equal(t, stableDevice, resolveConvergedInstallationID(accountWithoutDevice))
	require.False(t, applyCodexClientMetadata(body2, accountWithoutDevice))

	// 既有 client_metadata（如 turn metadata）保留，仅补 installation 键
	body3 := map[string]any{"client_metadata": map[string]any{"x-codex-turn-metadata": "t"}}
	require.True(t, applyCodexClientMetadata(body3, acc))
	cm3, _ := body3["client_metadata"].(map[string]any)
	require.Equal(t, "t", cm3["x-codex-turn-metadata"])
	require.Equal(t, "dev-xyz", cm3["x-codex-installation-id"])
}

// defaultCodexSynthInstructions：按模型选用真实 Codex base prompt。
func TestDefaultCodexSynthInstructionsModelAware(t *testing.T) {
	require.True(t, strings.Contains(defaultCodexSynthInstructions("gpt-5-codex"), "You are Codex, based on GPT-5"))
	require.True(t, strings.Contains(defaultCodexSynthInstructions("gpt-5.5"), "You are Codex, a coding agent based on GPT-5"))
	require.False(t, strings.Contains(defaultCodexSynthInstructions("gpt-5.5"), "You are GPT-5.1 running in the Codex CLI"))
	require.True(t, strings.Contains(defaultCodexSynthInstructions("gpt-5.2"), "You are GPT-5.2 running in the Codex CLI"))
	require.True(t, strings.Contains(defaultCodexSynthInstructions("gpt-5.1"), "You are GPT-5.1 running in the Codex CLI"))
}

func TestApplyInstructionsReplacesOnlyGenericFallback(t *testing.T) {
	model := "gpt-5.6-sol"
	generic := map[string]any{"model": model, "instructions": openai.DefaultInstructions}
	require.True(t, applyInstructions(generic, true))
	require.Equal(t, defaultCodexSynthInstructions(model), generic["instructions"])

	custom := map[string]any{"model": model, "instructions": "Keep the user's custom policy."}
	require.False(t, applyInstructions(custom, true))
	require.Equal(t, "Keep the user's custom policy.", custom["instructions"])
}
