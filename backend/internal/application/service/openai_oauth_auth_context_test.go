//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/stretchr/testify/require"
)

func desktopAuthContextToken(t *testing.T, plan string, expiresAt time.Time) string {
	t.Helper()
	payload := map[string]any{"https://api.openai.com/auth": map[string]any{
		"chatgpt_account_id": "workspace-a", "chatgpt_user_id": "user-a", "chatgpt_plan_type": plan,
	}}
	if !expiresAt.IsZero() {
		payload["exp"] = expiresAt.Unix()
	}
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(encoded) + ".fixture"
}

func desktopAuthContextAccount(plan, token string) *Account {
	return &Account{ID: 907, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{
			"access_token": token, "refresh_token": "fixture-refresh",
			"chatgpt_account_id": "workspace-a", "chatgpt_user_id": "user-a", "plan_type": plan,
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		}}
}

func TestOpenAIOAuthDesktopPlanCompatibility(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		changed       bool
	}{
		{"plus", "pro", false}, {"prolite", "promax", false},
		{"team", "business", false}, {"team", "self_serve_business_prolite", false},
		{"team", "self_serve_business_usage_based", false}, {"team", "ent26", false},
		{"team", "enterprise_cbp_automation", false}, {"team", "enterprise_cbp_usage_based", false},
		{"team", "enterprise", false}, {"team", "edu", false}, {"team", "edu_plus", false},
		{"team", "edu_pro", false}, {"team", "sci", false}, {"team", "sci_plus", false},
		{"team", "plus", true}, {"pro", "team", true}, {"team", "free", true},
		{"team", "future_workspace_plan", true}, {"team", "", false}, {"", "team", false},
	} {
		t.Run(tc.before+"/"+tc.after, func(t *testing.T) {
			account := desktopAuthContextAccount(tc.before, "opaque-fixture")
			token := desktopAuthContextToken(t, tc.after, time.Time{})
			err := validateOpenAIOAuthRequestPrincipal(account, token)
			require.Equal(t, tc.changed, isOpenAIOAuthPrincipalChangedError(err))
			if err != nil {
				require.NotContains(t, err.Error(), token)
			}
		})
	}
}

func TestOpenAITokenProvider_DesktopJWTExpiryRefresh(t *testing.T) {
	for _, plan := range []string{"plus", "pro", "team"} {
		for _, cached := range []bool{false, true} {
			for _, remaining := range []time.Duration{4 * time.Minute, -time.Minute} {
				t.Run(fmt.Sprintf("%s/cached=%t/remaining=%s", plan, cached, remaining), func(t *testing.T) {
					oldToken := desktopAuthContextToken(t, plan, time.Now().Add(remaining))
					account := desktopAuthContextAccount(plan, oldToken)
					repo := &refreshAPIAccountRepo{account: account}
					cache := newOpenAITokenCacheStub()
					if cached {
						cache.tokens[OpenAITokenCacheKey(account)] = oldToken
					}
					newToken := desktopAuthContextToken(t, plan, time.Now().Add(time.Hour))
					client := &tokenExpiredRecoveryOAuthClient{response: &openai.TokenResponse{
						AccessToken: newToken, RefreshToken: "rotated-fixture", ExpiresIn: 3600,
					}}
					oauth := NewOpenAIOAuthService(nil, client)
					t.Cleanup(oauth.Stop)
					provider := NewOpenAITokenProvider(repo, cache, oauth)
					provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauth, repo))
					token, err := provider.GetAccessToken(context.Background(), account)
					require.NoError(t, err)
					require.Equal(t, newToken, token)
					require.Equal(t, int32(1), atomic.LoadInt32(&client.refreshCalls))
					require.Equal(t, "rotated-fixture", repo.account.GetOpenAIRefreshToken())
				})
			}
		}
	}
}

func TestOpenAITokenProvider_DesktopRefreshPlanChange(t *testing.T) {
	for _, forced := range []bool{false, true} {
		t.Run(fmt.Sprintf("forced=%t", forced), func(t *testing.T) {
			account := desktopAuthContextAccount("team", desktopAuthContextToken(t, "team", time.Now().Add(time.Minute)))
			repo := &refreshAPIAccountRepo{account: account}
			cache := newOpenAITokenCacheStub()
			client := &tokenExpiredRecoveryOAuthClient{response: &openai.TokenResponse{
				AccessToken: desktopAuthContextToken(t, "plus", time.Now().Add(time.Hour)), RefreshToken: "rotated-fixture", ExpiresIn: 3600,
			}}
			oauth := NewOpenAIOAuthService(nil, client)
			t.Cleanup(oauth.Stop)
			provider := NewOpenAITokenProvider(repo, cache, oauth)
			provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauth, repo))
			var err error
			if forced {
				err = provider.RefreshAfterUnauthorized(context.Background(), account)
			} else {
				_, err = provider.GetAccessToken(context.Background(), account)
			}
			require.True(t, isOpenAIOAuthPrincipalChangedError(err), "%v", err)
			require.Equal(t, "plus", repo.account.GetCredential("plan_type"), "rotation must remain durable for later independent requests")
			require.Empty(t, cache.tokens[OpenAITokenCacheKey(account)])
		})
	}
}

func TestOpenAITokenProvider_DesktopNewerCachedJWT(t *testing.T) {
	account := desktopAuthContextAccount("team", desktopAuthContextToken(t, "team", time.Now().Add(-time.Minute)))
	account.Credentials["expires_at"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	cache := newOpenAITokenCacheStub()
	newToken := desktopAuthContextToken(t, "team", time.Now().Add(time.Hour))
	cache.tokens[OpenAITokenCacheKey(account)] = newToken
	token, err := NewOpenAITokenProvider(nil, cache, nil).GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, newToken, token, "an old row expiry must not invalidate a newer cached bearer")
}

func TestOpenAITokenProvider_DesktopExpiredJWTWithoutRefresh(t *testing.T) {
	account := desktopAuthContextAccount("team", desktopAuthContextToken(t, "team", time.Now().Add(-time.Minute)))
	account.Credentials["refresh_token"] = ""
	cache := newOpenAITokenCacheStub()
	cache.tokens[OpenAITokenCacheKey(account)] = account.GetOpenAIAccessToken()
	token, err := NewOpenAITokenProvider(nil, cache, nil).GetAccessToken(context.Background(), account)
	require.ErrorContains(t, err, "refresh_token is missing")
	require.Empty(t, token)
}

func TestOpenAIOAuthDesktopExpiryDoesNotExtendStoredDeadline(t *testing.T) {
	account := desktopAuthContextAccount("team", desktopAuthContextToken(t, "team", time.Now().Add(time.Hour)))
	storedExpiry := time.Now().Add(-time.Minute).Truncate(time.Second)
	account.Credentials["expires_at"] = storedExpiry.UTC().Format(time.RFC3339)
	require.Equal(t, storedExpiry.Unix(), openAIOAuthAccessTokenExpiresAt(account, account.GetOpenAIAccessToken()).Unix())
	require.True(t, (&OpenAITokenRefresher{}).NeedsRefresh(account, 5*time.Minute))
}

func TestOpenAIOAuthDesktopNewVersionsRetainEngineIdentity(t *testing.T) {
	for _, sample := range []struct{ version, platform string }{
		{"26.1103.41825", "Mac OS 26.1; arm64"},
		{"26.1103.32239", "Linux 6.8; x86_64"},
	} {
		ua := "codex_app_server_cli/0.167.0-alpha.3.2 (" + sample.platform + ") unknown (codex_app_server_cli; " + sample.version + ")"
		identity := resolveCodexOutboundIdentity(ua)
		require.Equal(t, ua, identity.userAgent)
		require.Equal(t, "0.167.0-alpha.3.2", identity.version)
		require.False(t, strings.Contains(identity.version, "26.1103"))
	}
}

type desktopAuthWaitingCache struct {
	*openAITokenCacheStub
	reads atomic.Int32
	token string
}

func (c *desktopAuthWaitingCache) GetAccessToken(context.Context, string) (string, error) {
	if c.reads.Add(1) == 1 {
		return "", nil
	}
	return c.token, nil
}

func TestOpenAITokenProvider_DesktopWaitedToken(t *testing.T) {
	for _, forced := range []bool{false, true} {
		for _, changed := range []string{"plan", "expiry"} {
			t.Run(fmt.Sprintf("forced=%t/%s", forced, changed), func(t *testing.T) {
				account := desktopAuthContextAccount("team", desktopAuthContextToken(t, "team", time.Now().Add(time.Minute)))
				plan, expiry := "plus", time.Now().Add(time.Hour)
				if changed == "expiry" {
					plan, expiry = "team", time.Now().Add(-time.Minute)
				}
				cache := &desktopAuthWaitingCache{openAITokenCacheStub: newOpenAITokenCacheStub(), token: desktopAuthContextToken(t, plan, expiry)}
				cache.lockAcquired = false
				repo := &refreshAPIAccountRepo{account: account}
				client := &tokenExpiredRecoveryOAuthClient{}
				oauth := NewOpenAIOAuthService(nil, client)
				t.Cleanup(oauth.Stop)
				provider := NewOpenAITokenProvider(repo, cache, oauth)
				provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), NewOpenAITokenRefresher(oauth, repo))
				var err error
				if forced {
					err = provider.RefreshAfterUnauthorized(context.Background(), account)
				} else {
					var token string
					token, err = provider.GetAccessToken(context.Background(), account)
					require.Empty(t, token)
				}
				if changed == "plan" {
					require.True(t, isOpenAIOAuthPrincipalChangedError(err), "%v", err)
				} else {
					require.ErrorContains(t, err, "expired")
				}
				require.Zero(t, atomic.LoadInt32(&client.refreshCalls), "a competing worker owns the refresh lock")
			})
		}
	}
}
