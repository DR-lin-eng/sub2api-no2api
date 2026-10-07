package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLiveBillingSurvivesExpiryAndFreezesDurationAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { require.NoError(t, client.Close()) }()
	first, ok := NewGatewayCache(client).(*gatewayCache)
	require.True(t, ok)
	second, ok := NewGatewayCache(client).(*gatewayCache)
	require.True(t, ok)
	ctx := context.Background()
	start := time.Unix(1000, 0)
	record := &service.LiveCallRecord{
		CallID: "rtc_voice", CallHash: HashLiveCallID("rtc_voice"), AccountID: 11, APIKeyID: 22, UserID: 33,
		CreatedAt: start, ExpiresAt: start.Add(time.Minute), Controller: service.LiveControllerPending,
		Model: service.CodexVoiceModel, CodexVoice: true, SessionID: "session", ThreadID: "thread",
		Billing: &service.LiveBillingSnapshot{RateMultiplier: 1, AccountRateMultiplier: 2, APIKeyQuota: true},
	}
	require.NoError(t, first.SaveLiveCall(ctx, record, time.Minute))
	server.FastForward(2 * time.Hour)
	loaded, err := second.GetLiveCall(ctx, record.CallHash)
	require.NoError(t, err, "unsettled invoices must not disappear on Redis TTL")
	require.True(t, loaded.CodexVoice)
	require.Equal(t, record.Billing, loaded.Billing)
	require.Equal(t, "session", loaded.SessionID)
	due, err := second.ListDueLiveCalls(ctx, start.Add(2*time.Minute), 32)
	require.NoError(t, err)
	require.Len(t, due, 1)
	frozen, err := second.PrepareLiveCallSettlement(ctx, record.CallHash, start.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, int64(60000), frozen.BillingDurationMs)
	require.Equal(t, service.LiveControllerClosed, frozen.Controller)
	retried, err := first.PrepareLiveCallSettlement(ctx, record.CallHash, start.Add(3*time.Hour))
	require.NoError(t, err)
	require.Equal(t, frozen.BillingDurationMs, retried.BillingDurationMs)
	require.NoError(t, first.MarkLiveCallBilled(ctx, record.CallHash, 24*time.Hour))
	due, err = second.ListDueLiveCalls(ctx, start.Add(4*time.Hour), 32)
	require.NoError(t, err)
	require.Empty(t, due)
	_, err = second.PrepareLiveCallSettlement(ctx, record.CallHash, start.Add(4*time.Hour))
	require.ErrorIs(t, err, service.ErrLiveCallNotFound)
	require.Positive(t, server.TTL(liveCallKey(record.CallHash)))
}

func TestLiveBillingProratesEarlyDisconnectAndRejectsInvalidSnapshot(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { require.NoError(t, client.Close()) }()
	cache, ok := NewGatewayCache(client).(*gatewayCache)
	require.True(t, ok)
	ctx := context.Background()
	start := time.Unix(1000, 0)
	record := &service.LiveCallRecord{CallID: "rtc_early", CallHash: HashLiveCallID("rtc_early"), CreatedAt: start, ExpiresAt: start.Add(time.Hour), Billing: &service.LiveBillingSnapshot{RateMultiplier: 1}}
	require.NoError(t, cache.SaveLiveCall(ctx, record, time.Hour))
	frozen, err := cache.PrepareLiveCallSettlement(ctx, record.CallHash, start.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(30000), frozen.BillingDurationMs)
	require.NoError(t, client.HSet(ctx, liveCallKey(record.CallHash), "billing_snapshot", "malformed").Err())
	_, err = cache.GetLiveCall(ctx, record.CallHash)
	require.Error(t, err)
}
