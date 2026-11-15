package handler

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func profitControlSlotContext(t *testing.T, gateway *service.OpenAIGatewayService, groupID int64) context.Context {
	t.Helper()
	group := &service.Group{
		ID:                   groupID,
		Platform:             service.PlatformOpenAI,
		Status:               service.StatusActive,
		Hydrated:             true,
		RateMultiplier:       1,
		SubscriptionType:     service.SubscriptionTypeStandard,
		ProfitControlEnabled: true,
		ProfitMinMargin:      0.5,
	}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	ctx, pricingAt := gateway.WithOpenAIRequestPricingContext(ctx, &groupID)
	require.False(t, pricingAt.IsZero())
	return ctx
}

func profitControlAcquiredSelection(accountID int64, rate float64, releases *atomic.Int64) *service.AccountSelectionResult {
	return &service.AccountSelectionResult{
		Account: &service.Account{
			ID:             accountID,
			Platform:       service.PlatformOpenAI,
			Type:           service.AccountTypeAPIKey,
			Status:         service.StatusActive,
			Schedulable:    true,
			Concurrency:    2,
			RateMultiplier: &rate,
		},
		Acquired: true,
		ReleaseFunc: func() {
			releases.Add(1)
		},
	}
}

func TestAcquireResponsesAccountSlotRechecksProfitBeforeForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(71)
	gateway := &service.OpenAIGatewayService{}

	t.Run("veto releases the acquired slot", func(t *testing.T) {
		var releases atomic.Int64
		handler := &OpenAIGatewayHandler{gatewayService: gateway}
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ginContext.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(profitControlSlotContext(t, gateway, groupID))
		streamStarted := false

		release, outcome := handler.acquireResponsesAccountSlot(
			ginContext,
			&groupID,
			"",
			profitControlAcquiredSelection(1, 0.8, &releases),
			false,
			&streamStarted,
			zap.NewNop(),
		)
		require.Equal(t, accountSlotAcquireProfitVeto, outcome)
		require.Nil(t, release)
		require.Zero(t, recorder.Body.Len())
		require.Equal(t, int64(1), releases.Load())
	})

	t.Run("eligible account remains admitted", func(t *testing.T) {
		var releases atomic.Int64
		handler := &OpenAIGatewayHandler{gatewayService: gateway}
		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		ginContext.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(profitControlSlotContext(t, gateway, groupID))
		streamStarted := false

		release, outcome := handler.acquireResponsesAccountSlot(
			ginContext,
			&groupID,
			"",
			profitControlAcquiredSelection(2, 0.3, &releases),
			false,
			&streamStarted,
			zap.NewNop(),
		)
		require.Equal(t, accountSlotAcquireReady, outcome)
		require.NotNil(t, release)
		release()
		require.Equal(t, int64(1), releases.Load())
	})
}
