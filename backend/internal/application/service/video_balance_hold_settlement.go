package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *OpenAIGatewayService) multimodalVideoCapture(ctx context.Context, c *gin.Context, account *Account, record *BalanceHoldRecord, seconds int) (*VideoBalanceCapture, error) {
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil || apiKey.ID != record.APIKeyID || apiKey.UserID != record.UserID {
		return nil, ErrBalanceHoldFailed.WithCause(fmt.Errorf("video settlement owner mismatch"))
	}
	if seconds <= 0 {
		seconds = record.DurationSeconds
	}
	seconds = NormalizeVideoBillingDurationSecondsOrDefault(seconds)
	requestedSeconds := NormalizeVideoBillingDurationSecondsOrDefault(record.DurationSeconds)
	ratio := math.Min(1, float64(seconds)/float64(requestedSeconds))
	amount := math.Min(record.HoldAmount, QuantizeUsageBillingAmount(record.HoldAmount*ratio))
	total := QuantizeUsageBillingAmount(record.BaseAmount * ratio)
	if total == 0 && amount > 0 {
		total = amount // Read compatibility for preview records without a snapshot.
	}
	accountRate := 1.0
	accountType := AccountTypeAPIKey
	if account != nil {
		if account.ID != record.AccountID {
			return nil, ErrBalanceHoldFailed.WithCause(fmt.Errorf("video settlement account mismatch"))
		}
		accountRate, accountType = account.BillingRateMultiplier(), account.Type
	}
	billingMode, resolution := string(BillingModeVideo), record.Resolution
	inbound, upstream := "/v1/videos/generations", "/v1/videos/generations"
	if c != nil && c.Request != nil && c.Request.URL != nil {
		inbound = c.Request.URL.Path
	}
	if actual := strings.TrimSpace(GetActualOpenAIUpstreamEndpoint(c)); actual != "" {
		upstream = actual
	}
	durationMs := int(time.Since(record.CreatedAt).Milliseconds())
	if record.CreatedAt.IsZero() || durationMs <= 0 {
		durationMs = seconds * 1000
	}
	groupID := record.GroupID
	log := &UsageLog{
		UserID: record.UserID, APIKeyID: record.APIKeyID, AccountID: record.AccountID,
		GroupID: &groupID, RequestID: record.UpstreamRequestID, Model: record.Model,
		RequestedModel: record.Model, InboundEndpoint: &inbound, UpstreamEndpoint: &upstream,
		VideoCount: 1, VideoResolution: &resolution, VideoDurationSeconds: &seconds,
		ImageOutputCost: total, TotalCost: total, ActualCost: amount,
		RateMultiplier: record.RateMultiplier, AccountRateMultiplier: &accountRate,
		BillingType: BillingTypeBalance, RequestType: RequestTypeSync, BillingMode: &billingMode,
		DurationMs: &durationMs, SessionID: optionalSessionID(ExtractClientSessionID(c)), CreatedAt: time.Now(),
	}
	if groupID <= 0 {
		log.GroupID = nil
	}
	if account != nil && groupID > 0 {
		applyAccountStatsCost(ctx, log, s.channelService, s.billingService,
			account.ID, groupID, account.GetMappedModel(record.Model), record.Model,
			UsageTokens{}, total)
	}
	cmd := &UsageBillingCommand{
		RequestID: BalanceCaptureRequestID(record.HoldKey), APIKeyID: record.APIKeyID,
		UserID: record.UserID, AccountID: record.AccountID, GroupID: record.GroupID,
		AccountType: accountType, Model: record.Model, BillingType: BillingTypeBalance,
		APIKeyAuthCacheKey: HashAPIKeyAuthCacheKey(apiKey.Key), MediaType: "video",
	}
	if apiKey.Quota > 0 {
		cmd.APIKeyQuotaCost = amount
	}
	if apiKey.HasRateLimits() {
		cmd.APIKeyRateLimitCost = amount
	}
	if account != nil && account.SupportsAccountQuota() && account.HasAnyQuotaLimit() {
		accountCost := total
		if log.AccountStatsCost != nil {
			accountCost = *log.AccountStatsCost
		}
		cmd.AccountQuotaCost = QuantizeUsageBillingAmount(accountCost * accountRate)
		cmd.AccountQuotaHasDailyLimit = account.GetQuotaDailyLimit() > 0
		cmd.AccountQuotaHasWeeklyLimit = account.GetQuotaWeeklyLimit() > 0
	}
	platform := QuotaPlatform(ctx, apiKey)
	if platform == PlatformComposite || platform == "" {
		platform = PlatformMultimodal
	}
	if s.billingCacheService != nil && s.billingCacheService.HasUserPlatformQuotaLimit(ctx, record.UserID, platform) {
		cmd.QuotaPlatform, cmd.UserPlatformQuotaCost = platform, amount
	}
	return &VideoBalanceCapture{HoldKey: record.HoldKey, Billing: cmd, UsageLog: log}, nil
}
