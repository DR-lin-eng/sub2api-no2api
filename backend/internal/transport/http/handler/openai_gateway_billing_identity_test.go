package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/stretchr/testify/require"
)

type cyberBillingIdentityRepo struct {
	service.UsageBillingRepository
	commands chan string
}

func (r *cyberBillingIdentityRepo) Apply(_ context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.commands <- cmd.RequestID
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

type cyberBillingIdentityLogRepo struct {
	service.UsageLogRepository
}

func (cyberBillingIdentityLogRepo) Create(context.Context, *service.UsageLog) (bool, error) {
	return true, nil
}

func TestCyberBillingPreservesPrivateRequestAndTurnIdentity(t *testing.T) {
	for _, websocket := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "websocket"}[websocket], func(t *testing.T) {
			repo := &cyberBillingIdentityRepo{commands: make(chan string, 2)}
			cfg := &config.Config{}
			cfg.Default.RateMultiplier = 1
			gateway := service.NewOpenAIGatewayService(nil, cyberBillingIdentityLogRepo{}, repo, nil, nil, nil, nil,
				cfg, nil, nil, service.NewBillingService(cfg, nil), nil, &service.BillingCacheService{}, nil,
				&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			h := &OpenAIGatewayHandler{gatewayService: gateway}
			c := newTestGinContext()
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			parent := context.WithValue(c.Request.Context(), ctxkey.UsageBillingRequestID, "server-request")
			parent, cancel := context.WithCancel(parent)
			cancel()
			c.Request = c.Request.WithContext(parent)
			c.Header("X-Request-ID", "attacker-fixed")
			apiKey := &service.APIKey{ID: 2, User: &service.User{ID: 1}}
			account := &service.Account{ID: 3}
			for turn := 1; turn <= 2; turn++ {
				service.MarkOpsCyberPolicy(c, service.CyberPolicyMark{Message: "flagged", UpstreamInTok: 1200, UpstreamOutTok: 300})
				want := "local:server-request"
				if websocket {
					usageCtx := openAIWSTurnUsageContext(parent, turn)
					turnID, ok := usageCtx.Value(ctxkey.UsageBillingRequestID).(string)
					require.True(t, ok)
					want = "local:" + turnID
					h.recordCyberPolicyIfMarkedWithUsageContext(usageCtx, c, apiKey, account, nil, "gpt-5.1", true, "", service.ChannelUsageFields{}, "")
				} else {
					h.recordCyberPolicyIfMarked(c, apiKey, account, nil, "gpt-5.1", true, "", service.ChannelUsageFields{}, "")
				}
				select {
				case got := <-repo.commands:
					require.Equal(t, want, got)
				case <-time.After(3 * time.Second):
					t.Fatal("cyber usage was not billed")
				}
				clearCyberPolicyTurnState(c)
			}
		})
	}
}
