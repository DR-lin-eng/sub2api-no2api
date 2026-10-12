//go:build integration

package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type billingIdentityUsageLogRepo struct {
	service.UsageLogRepository
	last *service.UsageLog
}

func (r *billingIdentityUsageLogRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.last = log
	return true, nil
}

type billingIdentityQuotaUpdater struct{}

func (billingIdentityQuotaUpdater) UpdateQuotaUsed(context.Context, int64, float64) error {
	return fmt.Errorf("atomic billing must own key quota updates")
}

func (billingIdentityQuotaUpdater) UpdateRateLimitUsage(context.Context, int64, float64) error {
	return fmt.Errorf("atomic billing must own key rate limit updates")
}

// Exercise real ingress metadata, both RecordUsage entry points and PostgreSQL
// settlement. Repeat identical correlations, provider IDs and payloads, then
// change usage/payload; each ingress must bill once, while settlement retries
// for that ingress must remain harmless.
func TestUsageBillingGatewayRequestIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{service.PlatformAnthropic, service.PlatformOpenAI} {
		for _, subscriptionMode := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/subscription=%t/stream=%t", platform, subscriptionMode, stream), func(t *testing.T) {
					ctx := context.Background()
					client := testEntClient(t)
					user := mustCreateUser(t, client, &service.User{Balance: 100})
					group := &service.Group{Name: "billing-identity-" + uuid.NewString(), Platform: platform, RateMultiplier: 1}
					if subscriptionMode {
						group.SubscriptionType = service.SubscriptionTypeSubscription
					}
					group = mustCreateGroup(t, client, group)
					apiKey := mustCreateApiKey(t, client, &service.APIKey{
						UserID: user.ID, GroupID: &group.ID, Quota: 100, RateLimit5h: 100,
					})
					apiKey.Group, apiKey.User = group, user
					account := mustCreateAccount(t, client, &service.Account{
						Name: "billing-identity-" + uuid.NewString(), Type: service.AccountTypeAPIKey,
						Extra: map[string]any{"quota_limit": float64(100)},
					})
					var subscription *service.UserSubscription
					if subscriptionMode {
						subscription = mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group.ID})
					}
					cfg := &config.Config{}
					cfg.Default.RateMultiplier = 1
					cache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
					t.Cleanup(cache.Stop)
					logs := &billingIdentityUsageLogRepo{}
					billing := NewUsageBillingRepository(client, integrationDB)
					pricing := service.NewBillingService(cfg, nil)
					gateway := service.NewGatewayService(nil, nil, logs, billing, nil, nil, nil, nil, cfg, nil, nil,
						pricing, nil, cache, nil, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
					openai := service.NewOpenAIGatewayService(nil, logs, billing, nil, nil, nil, nil, cfg, nil, nil,
						pricing, nil, cache, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
					var totalCost, accountCost float64
					var requestIDs []string
					router := gin.New()
					router.Use(middleware.RequestLogger(), middleware.ClientRequestID())
					router.POST("/v1/test", func(c *gin.Context) {
						payload, err := io.ReadAll(c.Request.Body)
						require.NoError(t, err)
						outputTokens := 10
						if strings.Contains(string(payload), "changed") {
							outputTokens = 25
						}
						var record func() error
						if platform == service.PlatformAnthropic {
							input := &service.RecordUsageInput{
								Result: &service.ForwardResult{
									RequestID: "provider-fixed", Model: "claude-sonnet-4", Stream: stream,
									Usage: service.ClaudeUsage{InputTokens: 1000, OutputTokens: outputTokens}, Duration: time.Second,
								},
								APIKey: apiKey, User: user, Account: account, Subscription: subscription,
								RequestPayloadHash: service.HashUsageRequestPayload(payload), APIKeyService: billingIdentityQuotaUpdater{},
							}
							record = func() error { return gateway.RecordUsage(c.Request.Context(), input) }
						} else {
							input := &service.OpenAIRecordUsageInput{
								Result: &service.OpenAIForwardResult{
									RequestID: "provider-fixed", Model: "gpt-5.1", Stream: stream,
									Usage: service.OpenAIUsage{InputTokens: 1000, OutputTokens: outputTokens}, Duration: time.Second,
								},
								APIKey: apiKey, User: user, Account: account, Subscription: subscription,
								RequestPayloadHash: service.HashUsageRequestPayload(payload), APIKeyService: billingIdentityQuotaUpdater{},
							}
							record = func() error { return openai.RecordUsage(c.Request.Context(), input) }
						}
						require.NoError(t, record())
						require.NotNil(t, logs.last)
						totalCost += service.QuantizeUsageBillingAmount(logs.last.ActualCost)
						accountCost += service.QuantizeUsageBillingAmount(logs.last.TotalCost)
						requestIDs = append(requestIDs, logs.last.RequestID)
						// A retry of settlement uses the same private ID and must not double bill.
						require.NoError(t, record())
						c.Status(http.StatusOK)
					})
					for _, payload := range []string{`{"input":"same"}`, `{"input":"same"}`, `{"input":"changed"}`} {
						req := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader(payload))
						req.Header.Set("X-Client-Request-ID", "anything")
						req.Header.Set("X-Request-ID", "anything")
						// Also exercise the inherited-context representation accepted by ingress.
						req = req.WithContext(context.WithValue(req.Context(), ctxkey.ClientRequestID, "anything"))
						w := httptest.NewRecorder()
						router.ServeHTTP(w, req)
						require.Equal(t, http.StatusOK, w.Code)
						require.Equal(t, "anything", w.Header().Get("X-Request-ID"))
					}
					require.Len(t, requestIDs, 3)
					require.NotEqual(t, requestIDs[0], requestIDs[1])
					require.NotEqual(t, requestIDs[1], requestIDs[2])
					require.Positive(t, totalCost)
					var balance, keyUsage, rateUsage, accountUsage float64
					require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id=$1", user.ID).Scan(&balance))
					require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used, usage_5h FROM api_keys WHERE id=$1", apiKey.ID).Scan(&keyUsage, &rateUsage))
					require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id=$1", account.ID).Scan(&accountUsage))
					require.InDelta(t, totalCost, keyUsage, 1e-8)
					require.InDelta(t, totalCost, rateUsage, 1e-8)
					require.InDelta(t, accountCost, accountUsage, 1e-8)
					if subscriptionMode {
						require.Equal(t, float64(100), balance)
						var daily, weekly, monthly float64
						require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd FROM user_subscriptions WHERE id=$1", subscription.ID).Scan(&daily, &weekly, &monthly))
						for _, usage := range []float64{daily, weekly, monthly} {
							require.InDelta(t, totalCost, usage, 1e-8)
						}
					} else {
						require.InDelta(t, 100-totalCost, balance, 1e-8)
						var daily, weekly, monthly float64
						require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd FROM user_platform_quotas WHERE user_id=$1 AND platform=$2", user.ID, platform).Scan(&daily, &weekly, &monthly))
						for _, usage := range []float64{daily, weekly, monthly} {
							require.InDelta(t, totalCost, usage, 1e-8)
						}
					}
					var dedup int
					require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id=$1", apiKey.ID).Scan(&dedup))
					require.Equal(t, 3, dedup)
					t.Logf("3 independent ingresses + 3 settlement retries: cost=%g key=%g account=%g dedup=%d", totalCost, keyUsage, accountUsage, dedup)
				})
			}
		}
	}
}
