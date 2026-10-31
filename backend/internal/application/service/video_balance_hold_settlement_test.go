//go:build unit

package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMultimodalVideoCaptureUsesSnapshotAndOrdinaryQuotaEffects(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{ID: 7, UserID: 42, Quota: 10, RateLimit5h: 10,
		Group: &Group{Platform: PlatformMultimodal, RateMultiplier: 100}})
	record := &BalanceHoldRecord{HoldKey: "snapshot", UserID: 42, APIKeyID: 7, AccountID: 99,
		DurationSeconds: 8, HoldAmount: 1, BaseAmount: 2, RateMultiplier: .5}
	account := &Account{ID: 99, Type: AccountTypeAPIKey, Extra: map[string]any{"quota_limit": float64(100)}}
	input, err := (&OpenAIGatewayService{}).multimodalVideoCapture(context.Background(), c, account, record, 4)
	require.NoError(t, err)
	require.InDelta(t, .5, input.UsageLog.ActualCost, 1e-8)
	require.InDelta(t, 1, input.UsageLog.TotalCost, 1e-8)
	require.InDelta(t, .5, input.UsageLog.RateMultiplier, 1e-8, "live group changes must not reprice accepted work")
	require.InDelta(t, .5, input.Billing.APIKeyQuotaCost, 1e-8)
	require.InDelta(t, .5, input.Billing.APIKeyRateLimitCost, 1e-8)
	require.InDelta(t, 1, input.Billing.AccountQuotaCost, 1e-8)
	require.Zero(t, input.Billing.BalanceCost, "capture already spends frozen funds")
}

func TestMultimodalVideoCaptureRejectsWrongOwner(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{ID: 8, UserID: 42})
	_, err := (&OpenAIGatewayService{}).multimodalVideoCapture(context.Background(), c, nil,
		&BalanceHoldRecord{UserID: 42, APIKeyID: 7}, 8)
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
}

func TestVideoBalanceSweepNeverRefundsBoundTaskByAge(t *testing.T) {
	repo := newReproVideoBillingRepo()
	record := reproExpiredHold(repo, "old-bound", .5)
	record.ExpiresAt = time.Now().Add(-72 * time.Hour)
	(&OpenAIGatewayService{usageBillingRepo: repo}).sweepExpiredVideoBalanceHolds(context.Background())
	require.Empty(t, repo.releaseCalls, "age does not prove that submitted upstream work failed")
	require.Equal(t, BalanceHoldStatusPending, record.Status)
}

func TestMultimodalVideoStatusMissingHoldFailsClosed(t *testing.T) {
	repo := newReproVideoBillingRepo()
	svc := &OpenAIGatewayService{usageBillingRepo: repo}
	body := []byte(`{"id":"unknown-task","status":"completed","url":"https://cdn.example.test/video.mp4"}`)
	rendered, err := svc.applyMultimodalVideoBilling(context.Background(), reproHoldModeContext(), nil,
		MultimodalMediaEndpointVideoStatus, "unknown-task", nil, body)
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
	require.Equal(t, body, rendered)
	require.Empty(t, repo.captureCalls)

	rendered, err = svc.applyMultimodalVideoBilling(context.Background(), reproSubscriptionContext(), nil,
		MultimodalMediaEndpointVideoStatus, "unknown-task", nil, body)
	require.NoError(t, err, "subscription requests legitimately have no balance hold")
	require.Equal(t, body, rendered)
}

func TestMultimodalVideoStatusReleasedHoldFailsClosed(t *testing.T) {
	repo := newReproVideoBillingRepo()
	record := reproExpiredHold(repo, "released-status", .5)
	record.Status = BalanceHoldStatusReleased
	body := []byte(`{"id":"upstream-released-status","status":"completed","url":"https://cdn.example.test/video.mp4"}`)
	rendered, err := (&OpenAIGatewayService{usageBillingRepo: repo}).applyMultimodalVideoBilling(
		context.Background(), reproHoldModeContext(), nil, MultimodalMediaEndpointVideoStatus,
		record.UpstreamRequestID, nil, body)
	require.ErrorIs(t, err, ErrBalanceHoldFailed)
	require.Equal(t, body, rendered)
	require.Empty(t, repo.captureCalls)
}
