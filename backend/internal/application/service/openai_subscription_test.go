package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestFetchChatGPTSubscriptionExpiresAt(t *testing.T) {
	const wantExpiresAt = "2026-06-10T02:52:15Z"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/subscriptions", r.URL.Path)
		require.Equal(t, "acc_123", r.URL.Query().Get("account_id"))
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type":    "plus",
			"active_until": wantExpiresAt,
			"will_renew":   true,
			"id":           "sub_123",
		})
	}))
	defer server.Close()

	oldURL := chatGPTSubscriptionsURL
	chatGPTSubscriptionsURL = server.URL + "/backend-api/subscriptions"
	t.Cleanup(func() { chatGPTSubscriptionsURL = oldURL })

	got := fetchChatGPTSubscriptionExpiresAt(context.Background(), func(_ context.Context, proxyURL string) (*req.Client, error) {
		return req.C().SetTimeout(5 * time.Second), nil
	}, "access-token", "", "acc_123")

	require.Equal(t, wantExpiresAt, got)
}

func TestFetchChatGPTAccountInfo_SkipsExpiredWorkspaceCandidate(t *testing.T) {
	expiredAt := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/accounts/check/v4-2023-04-27", r.URL.Path)
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": map[string]any{
				"org-expired-workspace": map[string]any{
					"account": map[string]any{
						"plan_type":  "self_serve_business_usage_based",
						"is_default": true,
					},
					"entitlement": map[string]any{
						"expires_at": expiredAt,
					},
				},
				"personal-account": map[string]any{
					"account": map[string]any{
						"plan_type": "free",
					},
				},
			},
		})
	}))
	defer server.Close()

	oldURL := chatGPTAccountsCheckURL
	chatGPTAccountsCheckURL = server.URL + "/backend-api/accounts/check/v4-2023-04-27"
	t.Cleanup(func() { chatGPTAccountsCheckURL = oldURL })

	got := fetchChatGPTAccountInfo(context.Background(), func(_ context.Context, proxyURL string) (*req.Client, error) {
		return req.C().SetTimeout(5 * time.Second), nil
	}, "access-token", "", "org-expired-workspace")

	require.NotNil(t, got)
	require.Equal(t, "free", got.PlanType)
	require.Empty(t, got.SubscriptionExpiresAt)
}

func TestFetchChatGPTAccountInfo_SkipsDeactivatedWorkspaceCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/accounts/check/v4-2023-04-27", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": map[string]any{
				"org-deactivated-workspace": map[string]any{
					"account": map[string]any{
						"plan_type":      "self_serve_business_usage_based",
						"is_default":     true,
						"is_deactivated": true,
					},
				},
				"personal-account": map[string]any{
					"account": map[string]any{
						"plan_type": "pro",
					},
				},
			},
		})
	}))
	defer server.Close()

	oldURL := chatGPTAccountsCheckURL
	chatGPTAccountsCheckURL = server.URL + "/backend-api/accounts/check/v4-2023-04-27"
	t.Cleanup(func() { chatGPTAccountsCheckURL = oldURL })

	got := fetchChatGPTAccountInfo(context.Background(), func(_ context.Context, proxyURL string) (*req.Client, error) {
		return req.C().SetTimeout(5 * time.Second), nil
	}, "access-token", "", "org-deactivated-workspace")

	require.NotNil(t, got)
	require.Equal(t, "pro", got.PlanType)
}

func TestShouldApplyChatGPTAccountInfoPlanType(t *testing.T) {
	require.False(t, shouldApplyChatGPTAccountInfoPlanType("pro", "self_serve_business_usage_based"))
	require.False(t, shouldApplyChatGPTAccountInfoPlanType("free", "team"))
	require.False(t, shouldApplyChatGPTAccountInfoPlanType("", ""))
	require.True(t, shouldApplyChatGPTAccountInfoPlanType("", "pro"))
}

func TestChatGPTAccountInfoBelongsToTokenAccount(t *testing.T) {
	require.False(t, chatGPTAccountInfoBelongsToTokenAccount(
		&OpenAITokenInfo{ChatGPTAccountID: "personal-a"}, &ChatGPTAccountInfo{AccountID: "workspace-b"}))
	require.True(t, chatGPTAccountInfoBelongsToTokenAccount(
		&OpenAITokenInfo{ChatGPTAccountID: "personal-a"}, &ChatGPTAccountInfo{AccountID: "PERSONAL-A"}))
	require.True(t, chatGPTAccountInfoBelongsToTokenAccount(
		&OpenAITokenInfo{}, &ChatGPTAccountInfo{AccountID: "workspace-b"}))
}

func TestFetchChatGPTAccountInfoReportsNestedAccountID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": map[string]any{
				"default": map[string]any{
					"account": map[string]any{
						"account_id": "personal-account-a", "plan_type": "plus", "is_default": true,
					},
				},
			},
		})
	}))
	defer server.Close()

	got := fetchChatGPTAccountInfo(context.Background(), newQuotaRedirectingFactory(server), "access-token", "", "")
	require.NotNil(t, got)
	require.Equal(t, "personal-account-a", got.AccountID)
}

func TestEnrichTokenInfoWorkspaceExpiryDoesNotOverridePersonalSubscription(t *testing.T) {
	const (
		personalID     = "personal-account-a"
		workspaceID    = "personal-workspace-b"
		personalExpiry = "2027-03-01T00:00:00Z"
	)
	workspaceExpiry := time.Now().Add(720 * time.Hour).UTC().Format(time.RFC3339)
	subscriptionCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/accounts/check/v4-2023-04-27":
			_ = json.NewEncoder(w).Encode(map[string]any{"accounts": map[string]any{
				workspaceID: map[string]any{
					"account":     map[string]any{"account_id": workspaceID, "plan_type": "pro", "is_default": true},
					"entitlement": map[string]any{"expires_at": workspaceExpiry},
				},
			}})
		case "/backend-api/subscriptions":
			subscriptionCalls++
			require.Equal(t, personalID, r.URL.Query().Get("account_id"))
			_ = json.NewEncoder(w).Encode(map[string]any{"active_until": personalExpiry})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer server.Close()

	tokenInfo := &OpenAITokenInfo{
		AccessToken: "access-token", ChatGPTAccountID: personalID, OrganizationID: workspaceID, PlanType: "pro",
	}
	(&OpenAIOAuthService{privacyClientFactory: newQuotaRedirectingFactory(server)}).enrichTokenInfo(context.Background(), tokenInfo, "")
	require.Equal(t, personalExpiry, tokenInfo.SubscriptionExpiresAt)
	require.NotEqual(t, workspaceExpiry, tokenInfo.SubscriptionExpiresAt)
	require.Equal(t, 1, subscriptionCalls)
}

func TestEnrichTokenInfoMatchingAccountKeepsEntitlementWithoutExtraLookup(t *testing.T) {
	const accountID = "personal-account-a"
	expiry := time.Now().Add(720 * time.Hour).UTC().Format(time.RFC3339)
	subscriptionCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/accounts/check/v4-2023-04-27":
			_ = json.NewEncoder(w).Encode(map[string]any{"accounts": map[string]any{
				accountID: map[string]any{
					"account":     map[string]any{"account_id": accountID, "plan_type": "plus", "is_default": true},
					"entitlement": map[string]any{"expires_at": expiry},
				},
			}})
		case "/backend-api/subscriptions":
			subscriptionCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer server.Close()

	tokenInfo := &OpenAITokenInfo{
		AccessToken: "access-token", ChatGPTAccountID: accountID, OrganizationID: accountID, PlanType: "plus",
	}
	(&OpenAIOAuthService{privacyClientFactory: newQuotaRedirectingFactory(server)}).enrichTokenInfo(context.Background(), tokenInfo, "")
	require.Equal(t, expiry, tokenInfo.SubscriptionExpiresAt)
	require.Zero(t, subscriptionCalls)
}
