package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIServerTokenUsageUnmarshalAppServerResponse(t *testing.T) {
	body := []byte(`{
      "id": 7,
      "result": {
        "summary": {
          "lifetimeTokens": 1234567,
          "peakDailyTokens": 45678,
          "longestRunningTurnSec": 540,
          "currentStreakDays": 8,
          "longestStreakDays": 14
        },
        "dailyUsageBuckets": [{"startDate":"2026-06-18","tokens":12345}],
        "currentResetCycleTokens": 777,
        "currentResetCycleWindowMinutes": 10080,
        "currentResetCycleLimitId": "codex",
        "currentResetCycleApproximate": false
      }
    }`)
	var usage OpenAIServerTokenUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.NotNil(t, usage.Summary.LifetimeTokens)
	require.Equal(t, int64(1234567), *usage.Summary.LifetimeTokens)
	require.Equal(t, int64(540), *usage.Summary.LongestRunningTurnSeconds)
	require.Equal(t, []OpenAITokenUsageDailyBucket{{StartDate: "2026-06-18", Tokens: 12345}}, usage.DailyUsageBuckets)
	require.Equal(t, int64(777), *usage.CurrentResetCycleTokens)
	require.Equal(t, int64(10080), usage.CurrentResetCycleWindowMinutes)
}

func TestOpenAIServerTokenUsageUnmarshalProfileStats(t *testing.T) {
	body := []byte(`{
      "stats": {
        "lifetime_tokens": 99,
        "peak_daily_tokens": 11,
        "longest_running_turn_sec": 7,
        "current_streak_days": 2,
        "longest_streak_days": 3,
        "daily_usage_buckets": [{"start_date":"2026-08-20","tokens":88}]
      }
    }`)
	var usage OpenAIServerTokenUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.Equal(t, int64(99), *usage.Summary.LifetimeTokens)
	require.Equal(t, int64(7), *usage.Summary.LongestRunningTurnSeconds)
	require.Equal(t, int64(88), usage.DailyUsageBuckets[0].Tokens)
}

func TestOpenAIQuotaUsagePreservesUnknownBucketFields(t *testing.T) {
	body := []byte(`{
      "rateLimitsByLimitId": {
        "gpt-aaa": 100,
        "future": {"limitId":"future","windowDurationMins":"unknown","newField":"keep-me"}
      }
    }`)
	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.Equal(t, float64(100), usage.RateLimitsByLimitID["gpt-aaa"].RawValue)
	require.Equal(t, "keep-me", usage.RateLimitsByLimitID["future"].RawFields["newField"])

	var snake OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"rate_limits_by_limit_id":{"gpt-aaa":100}}`), &snake))
	require.Equal(t, float64(100), snake.RateLimitsByLimitID["gpt-aaa"].RawValue)

	var direct OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"rateLimits":{"limitId":"gpt-aaa","newField":"direct"}}`), &direct))
	require.Equal(t, "direct", direct.RateLimitsByLimitID["gpt-aaa"].RawFields["newField"])

	var malformedWindow OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"rateLimitsByLimitId":{"gpt-bbb":{"limitId":"gpt-bbb","primary":{"futureWindow":"keep"}}}}`), &malformedWindow))
	primaryRaw, ok := malformedWindow.RateLimitsByLimitID["gpt-bbb"].RawFields["primary"]
	require.True(t, ok)
	primaryFields, ok := primaryRaw.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "keep", primaryFields["futureWindow"])
}

func TestQueryUsageWithServerUsageFetchesProfileCounters(t *testing.T) {
	account := &Account{
		ID:       501,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "account-501",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "fake-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/wham/usage"):
			_, _ = w.Write([]byte(`{"rateLimitsByLimitId":{"codex":{"primary":{"usedPercent":12,"windowDurationMins":15,"resetsAt":1730947200}}}}`))
		case strings.HasSuffix(r.URL.Path, "/wham/rate-limit-reset-credits"):
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/wham/profiles/me"):
			_, _ = w.Write([]byte(`{"stats":{"lifetime_tokens":1234,"daily_usage_buckets":[{"start_date":"2026-08-23","tokens":321}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(srv))
	usage, err := svc.QueryUsageWithServerUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, usage.ServerTokenUsage)
	require.Equal(t, int64(1234), *usage.ServerTokenUsage.Summary.LifetimeTokens)
	require.Equal(t, int64(321), usage.ServerTokenUsage.DailyUsageBuckets[0].Tokens)
}

func TestEnrichServerTokenUsageResetCycleSumsDailyBuckets(t *testing.T) {
	resetAt := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	usage := &OpenAIServerTokenUsage{
		DailyUsageBuckets: []OpenAITokenUsageDailyBucket{
			{StartDate: "2026-08-23", Tokens: 100},
			{StartDate: "2026-08-24", Tokens: 200},
			{StartDate: "2026-08-30", Tokens: 999},
		},
	}
	quota := &OpenAIQuotaUsage{RateLimitsByLimitID: map[string]OpenAIAppServerRateLimitBucket{
		"codex": {
			LimitID: "codex",
			Primary: &OpenAIAppServerRateLimitWindow{
				WindowDurationMins: 7 * 24 * 60,
				ResetsAt:           resetAt.Unix(),
			},
		},
	}}
	enrichServerTokenUsageResetCycle(usage, quota, time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC))
	require.NotNil(t, usage.CurrentResetCycleTokens)
	require.Equal(t, int64(300), *usage.CurrentResetCycleTokens)
	require.Equal(t, int64(10080), usage.CurrentResetCycleWindowMinutes)
	require.Equal(t, "codex", usage.CurrentResetCycleLimitID)
	require.False(t, usage.CurrentResetCycleApproximate)
}
