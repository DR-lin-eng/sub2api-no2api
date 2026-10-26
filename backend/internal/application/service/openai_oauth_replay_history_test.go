package service

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"strings"
	"testing"
)

func reasoningReplayTestAccount(oauth bool) *Account {
	if oauth {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	}
	return &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
}

func TestNormalizeOpenAIOAuthReplayHistory_Content(t *testing.T) {
	const reasoning = `{"type":"reasoning","summary":[{"type":"summary_text","text":"keep summary"}],"content":[{"type":"reasoning_text","text":"response-only"}],"encrypted_content":"keep cipher","nonce":9007199254740993}`
	for _, single := range []bool{false, true} {
		for _, oauth := range []bool{false, true} {
			t.Run(fmt.Sprintf("single=%v/oauth=%v", single, oauth), func(t *testing.T) {
				input := reasoning
				path := "input"
				if !single {
					input = "[" + reasoning + "]"
					path = "input.0"
				}
				body := []byte(`{"nonce":9007199254740993,"input":` + input + `}`)
				original := append([]byte(nil), body...)
				sanitized, changed, err := normalizeOpenAIOAuthReplayHistoryForAccount(body, reasoningReplayTestAccount(oauth), false, false)
				require.NoError(t, err)
				require.Equal(t, oauth, changed)
				require.Equal(t, original, body)
				require.Equal(t, "9007199254740993", gjson.GetBytes(sanitized, "nonce").Raw)
				require.Equal(t, "9007199254740993", gjson.GetBytes(sanitized, path+".nonce").Raw)
				require.Equal(t, "keep summary", gjson.GetBytes(sanitized, path+".summary.0.text").String())
				require.Equal(t, "keep cipher", gjson.GetBytes(sanitized, path+".encrypted_content").String())
				if oauth {
					require.False(t, gjson.GetBytes(sanitized, path+".content").Exists())
				} else {
					require.Equal(t, original, sanitized)
				}
				again, changedAgain, err := normalizeOpenAIOAuthReplayHistoryForAccount(sanitized, reasoningReplayTestAccount(oauth), false, false)
				require.NoError(t, err)
				require.False(t, changedAgain)
				require.Equal(t, sanitized, again)
			})
		}
	}
}

func TestNormalizeOpenAIOAuthReplayHistory_PreservesOtherContent(t *testing.T) {
	for _, body := range []string{
		`{"input":[{"type":"reasoning","summary":[],"content":[]}]}`,
		`{"input":[{"type":"reasoning","summary":[],"content":null}]}`,
		`{"input":[{"type":"reasoning","summary":[]}]}`,
		`{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"keep"}]}]}`,
		`{"input":[{"type":"function_call_output","content":[{"type":"reasoning_text","text":"tool data"}],"output":"keep"}]}`,
		`{"input":[{"type":"message","role":"user","content":[{"type":"reasoning","content":["nested user data"]}]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			sanitized, changed, err := normalizeOpenAIOAuthReplayHistoryForAccount([]byte(body), reasoningReplayTestAccount(true), false, false)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, string(sanitized))
		})
	}
}

func TestNormalizeOpenAIOAuthReplayHistory_StripsEveryReasoningContent(t *testing.T) {
	items := make([]string, 20)
	for i := range items {
		items[i] = `{"type":"reasoning","id":"rs_test","content":[{"type":"reasoning_text","text":"response-only"}],"summary":[],"encrypted_content":"cipher"}`
	}
	body := []byte(`{"input":[` + strings.Join(items, ",") + `]}`)
	sanitized, changed, err := normalizeOpenAIOAuthReplayHistoryForAccount(body, reasoningReplayTestAccount(true), false, false)
	require.NoError(t, err)
	require.True(t, changed)
	result := gjson.GetBytes(sanitized, "input").Array()
	require.Len(t, result, len(items))
	for _, item := range result {
		require.False(t, item.Get("content").Exists())
		require.Equal(t, "rs_test", item.Get("id").String(), "content normalization must not change ID policy")
		require.Equal(t, "cipher", item.Get("encrypted_content").String())
		require.True(t, item.Get("summary").IsArray())
	}
}
