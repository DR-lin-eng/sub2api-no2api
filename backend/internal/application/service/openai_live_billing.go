package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

const CodexVoiceModel = "gpt-live-1-codex"

// IsCodexVoiceSession matches the V3 model used by the stable Codex CLI. Other
// Live sessions retain the existing ChatGPT Desktop DeviceCheck requirement.
func IsCodexVoiceSession(session json.RawMessage) bool {
	return gjson.GetBytes(session, "model").String() == CodexVoiceModel
}

func (s *OpenAIGatewayService) LiveBillingSnapshotForAPIKey(ctx context.Context, key *APIKey) *LiveBillingSnapshot {
	rate := 1.0
	if key.Group != nil && key.GroupID != nil {
		rate = s.ResolveUserGroupRateMultiplier(ctx, key.UserID, *key.GroupID, key.Group.RateMultiplier)
	}
	quota := s.billingCacheService != nil && s.billingCacheService.HasUserPlatformQuotaLimit(ctx, key.UserID, PlatformOpenAI)
	return &LiveBillingSnapshot{
		RateMultiplier: rate, APIKeyQuota: key.Quota > 0,
		APIKeyRateLimit: key.HasRateLimits(), APIKeyAuthCacheKey: HashAPIKeyAuthCacheKey(key.Key),
		UserPlatformQuota: quota,
	}
}

func liveMinuteCost(durationMs int64, multiplier float64) *CostBreakdown {
	if durationMs < 0 {
		durationMs = 0
	}
	if multiplier < 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		multiplier = 0
	}
	base := decimal.NewFromInt(durationMs).Mul(decimal.RequireFromString("0.05")).Div(decimal.NewFromInt(60000))
	total, _ := base.Round(UsageBillingMonetaryScale).Float64()
	actual, _ := base.Mul(decimal.NewFromFloat(multiplier)).Round(UsageBillingMonetaryScale).Float64()
	return &CostBreakdown{TotalCost: total, ActualCost: actual, BillingMode: string(BillingModeLive)}
}

func (s *OpenAIGatewayService) finalizeBillableLiveCall(record *LiveCallRecord) {
	store, ok := s.cache.(LiveCallBillingStore)
	if !ok {
		return
	}
	ctx, cancel := detachedBillingContext(context.Background())
	defer cancel()
	frozen, err := store.PrepareLiveCallSettlement(ctx, record.CallHash, time.Now())
	if errors.Is(err, ErrLiveCallNotFound) {
		return
	}
	// The lease is no longer needed, even while a durable invoice waits for retry.
	s.releaseLiveLease(record.AccountID, record.UserID, record.APIKeyID, record.LeaseID)
	if err == nil {
		err = s.settleLiveCall(ctx, frozen)
	}
	if err != nil {
		logger.LegacyPrintf("service.openai_live", "Live settlement pending: call_hash=%s err=%v", record.CallHash, err)
	}
}

func (s *OpenAIGatewayService) settleLiveCall(ctx context.Context, record *LiveCallRecord) error {
	store, ok := s.cache.(LiveCallBillingStore)
	if !ok {
		return ErrLiveUnavailable
	}
	if record == nil || record.Billing == nil || s.usageBillingRepo == nil || s.usageLogRepo == nil {
		return ErrLiveUnavailable
	}
	snapshot := record.Billing
	cost := liveMinuteCost(record.BillingDurationMs, snapshot.RateMultiplier)
	billingType := int8(BillingTypeBalance)
	if record.SubscriptionID > 0 {
		billingType = BillingTypeSubscription
	}
	duration := int(record.BillingDurationMs)
	mode := string(BillingModeLive)
	usage := &UsageLog{
		UserID: record.UserID, APIKeyID: record.APIKeyID, AccountID: record.AccountID,
		RequestID: record.CallHash, Model: record.Model, RequestedModel: record.Model,
		GroupID: liveOptionalID(record.GroupID), SubscriptionID: liveOptionalID(record.SubscriptionID),
		TotalCost: cost.TotalCost, ActualCost: cost.ActualCost, RateMultiplier: snapshot.RateMultiplier,
		AccountRateMultiplier: &snapshot.AccountRateMultiplier, BillingType: billingType,
		RequestType: RequestTypeLive, BillingMode: &mode, DurationMs: &duration,
		UserAgent: optionalTrimmedStringPtr(record.UserAgent), IPAddress: optionalTrimmedStringPtr(record.IPAddress),
		InboundEndpoint:  optionalTrimmedStringPtr(record.InboundEndpoint),
		UpstreamEndpoint: optionalTrimmedStringPtr("/backend-api/codex/realtime/calls"), CreatedAt: record.CreatedAt,
	}
	key := &APIKey{ID: record.APIKeyID, UserID: record.UserID, GroupID: usage.GroupID}
	if snapshot.APIKeyRateLimit {
		key.RateLimit5h = 1
	}
	p := &postUsageBillingParams{
		Cost: cost, User: &User{ID: record.UserID}, APIKey: key,
		Account:            &Account{ID: record.AccountID, Type: AccountTypeOAuth, Platform: PlatformOpenAI},
		IsSubscriptionBill: record.SubscriptionID > 0, AccountRateMultiplier: snapshot.AccountRateMultiplier,
		Platform: PlatformOpenAI, DurablePlatformQuota: snapshot.UserPlatformQuota,
	}
	if record.SubscriptionID > 0 {
		p.Subscription = &UserSubscription{ID: record.SubscriptionID}
	}
	cmd := buildUsageBillingCommand(record.CallHash, usage, p)
	cmd.APIKeyAuthCacheKey = snapshot.APIKeyAuthCacheKey
	if snapshot.APIKeyQuota {
		cmd.APIKeyQuotaCost = cost.ActualCost
	}
	if snapshot.APIKeyRateLimit {
		cmd.APIKeyRateLimitCost = cost.ActualCost
	}
	if snapshot.UserPlatformQuota && record.SubscriptionID == 0 {
		cmd.QuotaPlatform = PlatformOpenAI
		cmd.UserPlatformQuotaCost = cost.ActualCost
	}
	cmd.Normalize()
	result, err := s.usageBillingRepo.Apply(ctx, cmd)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("Live billing returned no result")
	}
	if result.Applied {
		finalizePostUsageBilling(ctx, p, s.billingDeps(), result)
	}
	// Keep the Redis invoice discoverable until the usage row is also durable.
	if _, err := s.usageLogRepo.Create(ctx, usage); err != nil {
		return err
	}
	return store.MarkLiveCallBilled(ctx, record.CallHash, liveClosedRecordTTL)
}

// StartLiveBillingRecovery uses a fixed-size Redis batch rather than retaining
// disconnected sessions in an in-memory retry queue. Expired calls are included
// so another node can finish a call whose controller process exited.
func (s *OpenAIGatewayService) StartLiveBillingRecovery(ctx context.Context) {
	if _, ok := s.cache.(LiveCallBillingStore); !ok || s.usageBillingRepo == nil {
		return
	}
	s.liveBillingOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		s.liveBillingCancel = cancel
		go func() {
			ticker := time.NewTicker(liveLeaseRefreshInterval)
			defer ticker.Stop()
			for {
				s.recoverLiveBilling(runCtx)
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (s *OpenAIGatewayService) recoverLiveBilling(ctx context.Context) {
	store, ok := s.cache.(LiveCallBillingStore)
	if !ok {
		return
	}
	batchCtx, cancel := context.WithTimeout(ctx, liveRedisOperationTimeout)
	records, err := store.ListDueLiveCalls(batchCtx, time.Now(), 32)
	cancel()
	if err != nil {
		logger.LegacyPrintf("service.openai_live", "List pending Live settlements failed: %v", err)
		return
	}
	for _, record := range records {
		if ctx.Err() != nil {
			return
		}
		s.finalizeBillableLiveCall(record)
	}
}
