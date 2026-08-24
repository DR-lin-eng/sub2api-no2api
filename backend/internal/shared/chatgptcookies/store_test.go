package chatgptcookies

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJarKeepsOnlyCloudflareCookiesOnChatGPTHosts(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	jar := NewJar()
	u, err := url.Parse("https://chatgpt.com/backend-api/codex/responses")
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{
		{Name: "__cf_bm", Value: "bot"},
		{Name: "cf_clearance", Value: "clear"},
		{Name: "session_token", Value: "must-not-store"},
	})
	cookies := jar.Cookies(u)
	require.Len(t, cookies, 2)
	require.True(t, AllowedName(cookies[0].Name))
	require.True(t, AllowedName(cookies[1].Name))
}

func TestJarRejectsNonChatGPTAndPlainHTTP(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	jar := NewJar()
	for _, raw := range []string{"https://api.openai.com/v1/responses", "http://chatgpt.com/backend-api/codex/responses"} {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		jar.SetCookies(u, []*http.Cookie{{Name: "__cf_bm", Value: "x"}})
		require.Empty(t, jar.Cookies(u))
	}
}

func TestJarExpiresCloudflareCookie(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	jar := NewJar()
	u, err := url.Parse("https://chatgpt.com/")
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{{Name: "_cfuvid", Value: "visitor"}})
	require.Len(t, jar.Cookies(u), 1)
	jar.SetCookies(u, []*http.Cookie{{Name: "_cfuvid", MaxAge: -1}})
	require.Empty(t, jar.Cookies(u))
}

func TestJarKeepsHostAndPathIsolation(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	jar := NewJar()
	chatGPT, err := url.Parse("https://chatgpt.com/backend-api/codex/responses")
	require.NoError(t, err)
	chatOpenAI, err := url.Parse("https://chat.openai.com/backend-api/codex/responses")
	require.NoError(t, err)
	chatGPTAdmin, err := url.Parse("https://chatgpt.com/admin/panel")
	require.NoError(t, err)

	jar.SetCookies(chatGPT, []*http.Cookie{
		{Name: "__cf_bm", Value: "chatgpt-host", Path: "/backend-api"},
		{Name: "cf_clearance", Value: "chatgpt-root", Path: "/"},
	})
	jar.SetCookies(chatOpenAI, []*http.Cookie{{Name: "__cf_bm", Value: "chat-openai-host"}})

	cookies := jar.Cookies(chatGPT)
	require.Len(t, cookies, 2)
	require.Equal(t, "__cf_bm", cookies[0].Name)
	require.Equal(t, "chatgpt-host", cookies[0].Value)
	require.Equal(t, "cf_clearance", cookies[1].Name)
	require.Equal(t, "chatgpt-root", cookies[1].Value)

	adminCookies := jar.Cookies(chatGPTAdmin)
	require.Len(t, adminCookies, 1)
	require.Equal(t, "cf_clearance", adminCookies[0].Name)
	openAICookies := jar.Cookies(chatOpenAI)
	require.Len(t, openAICookies, 1)
	require.Equal(t, "chat-openai-host", openAICookies[0].Value, "host-only cookie must not be copied across ChatGPT hosts")
}

func TestJarSupportsExplicitChatGPTDomainCookieOnSubdomain(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	jar := NewJar()
	root, err := url.Parse("https://chatgpt.com/")
	require.NoError(t, err)
	subdomain, err := url.Parse("https://backend.chatgpt.com/backend-api/codex/responses")
	require.NoError(t, err)
	jar.SetCookies(root, []*http.Cookie{
		{Name: "cf_clearance", Value: "domain-cookie", Domain: ".chatgpt.com", Path: "/"},
	})
	cookies := jar.Cookies(subdomain)
	require.Len(t, cookies, 1)
	require.Equal(t, "domain-cookie", cookies[0].Value)
}

func TestObservableJarTracksRejectedAndExpiredCookiesWithoutExposingValues(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	jar := NewObservableJar()
	u, err := url.Parse("https://chatgpt.com/backend-api/codex/responses")
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{
		{Name: "session_token", Value: "secret"},
		{Name: "cf_clearance", Value: "clear"},
	})
	jar.SetCookies(u, []*http.Cookie{{Name: "cf_clearance", MaxAge: -1}})
	stats := jar.Stats()
	require.Equal(t, 0, stats.Stored)
	require.Equal(t, uint64(1), stats.Rejected)
	require.Equal(t, uint64(1), stats.Expired)
	jar.Clear()
	require.Empty(t, jar.Cookies(u))
}
