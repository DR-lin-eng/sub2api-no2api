//go:build unit

package service

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestNormalizeStrictChatDeveloperRoles_OpenCodeModelRouting(t *testing.T) {
	for _, tc := range []struct {
		name, platform, target, model, wantRole string
	}{
		{"go_deepseek", PlatformOpenCodeGo, DefaultOpenCodeGoBaseURL, "deepseek-v4.1-flash", "system"},
		{"go_deepseek_proxy", PlatformOpenCodeGo, "https://relay.example.test/v1/chat/completions", "deepseek-v4.1-flash", "system"},
		{"zen_deepseek", PlatformOpenAI, DefaultOpenCodeZenBaseURL, "deepseek-v4.1-flash", "system"},
		{"prefixed_deepseek", PlatformOpenAI, DefaultOpenCodeGoBaseURL, "opencode-go/DeepSeek-V4-Pro", "system"},
		{"go_other_model", PlatformOpenCodeGo, DefaultOpenCodeGoBaseURL, "gpt-5.6-luna", "developer"},
		{"zen_other_model", PlatformOpenAI, DefaultOpenCodeZenBaseURL, "gpt-5.6-luna", "developer"},
		{"unrelated_deepseek", PlatformOpenAI, "https://relay.example.test/v1/chat/completions", "deepseek-v4.1-flash", "developer"},
		{"host_suffix", PlatformOpenAI, "https://opencode.ai.example.test/v1/chat/completions", "deepseek-v4.1-flash", "developer"},
		{"host_in_path", PlatformOpenAI, "https://example.test/opencode.ai/zen/v1/chat/completions", "deepseek-v4.1-flash", "developer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A stale configured URL must not override the selected protocol destination.
			account := &Account{Platform: tc.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": DefaultOpenCodeZenBaseURL}}
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"developer","content":"rules"}]}`, tc.model))
			original := bytes.Clone(body)
			got, err := normalizeStrictChatDeveloperRoles(account, tc.target, body)
			require.NoError(t, err)
			require.Equal(t, tc.wantRole, gjson.GetBytes(got, "messages.0.role").String())
			require.Equal(t, original, body)
			if tc.wantRole == "developer" {
				require.Equal(t, original, got)
			}
		})
	}
}
