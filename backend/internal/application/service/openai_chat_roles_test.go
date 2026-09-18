//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeStrictChatDeveloperRolesPreservesPayload(t *testing.T) {
	body := []byte(`{"model":"alias","messages":[{"role":"system","content":"first"},{"role":"developer","content":[{"type":"text","text":"rules"}],"name":"policy"},{"role":"user","content":"hi"}],"metadata":{"large":9007199254740993}}`)
	got, err := normalizeStrictChatDeveloperRoles(&Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, "https://relay.example.test/v1/chat/completions", body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"alias","messages":[{"role":"system","content":"first"},{"role":"system","content":[{"type":"text","text":"rules"}],"name":"policy"},{"role":"user","content":"hi"}],"metadata":{"large":9007199254740993}}`, string(got))
	require.Equal(t, `{"model":"alias","messages":[{"role":"system","content":"first"},{"role":"developer","content":[{"type":"text","text":"rules"}],"name":"policy"},{"role":"user","content":"hi"}],"metadata":{"large":9007199254740993}}`, string(body))
	second, err := normalizeStrictChatDeveloperRoles(&Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}, "https://relay.example.test/v1/chat/completions", got)
	require.NoError(t, err)
	require.Equal(t, got, second)
}

func TestNormalizeStrictChatDeveloperRolesUsesExactDestination(t *testing.T) {
	body := []byte(`{"messages":[{"role":"developer","content":"rules"}]}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, target := range []string{
		"https://api.deepseek.com/v1/chat/completions",
		"https://API.KIMI.COM:443/coding/v1/chat/completions",
		"https://open.bigmodel.cn/api/paas/v4/chat/completions",
	} {
		got, err := normalizeStrictChatDeveloperRoles(account, target, body)
		require.NoError(t, err)
		require.Contains(t, string(got), `"role":"system"`)
	}
	for _, target := range []string{
		"https://api.openai.com/v1/chat/completions",
		"https://api.deepseek.com.example.test/v1/chat/completions",
		"https://api.deepseek.com@example.test/v1/chat/completions",
	} {
		got, err := normalizeStrictChatDeveloperRoles(account, target, body)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
}

func TestNormalizeStrictChatDeveloperRolesSkipsNonChatAccounts(t *testing.T) {
	body := []byte(`{"messages":[{"role":"developer","content":"rules"}]}`)
	for _, tc := range []struct {
		account *Account
		target  string
	}{
		{nil, "https://api.deepseek.com/v1/chat/completions"},
		{&Account{Platform: PlatformDeepseek, Type: AccountTypeOAuth}, "https://api.deepseek.com/v1/chat/completions"},
		{&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "https://compatible.example.test/v1/chat/completions"},
	} {
		got, err := normalizeStrictChatDeveloperRoles(tc.account, tc.target, body)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
}

func TestNormalizeStrictChatDeveloperRolesRejectsMalformedMessages(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	for _, body := range []string{`{"messages":"secret"}`, `{"messages":[null]}`, `{"messages":[{"role":12}]}`} {
		_, err := normalizeStrictChatDeveloperRoles(account, "https://api.deepseek.com/v1/chat/completions", []byte(body))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}
