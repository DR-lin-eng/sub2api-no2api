package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const (
	// videoBalanceHoldTTL 是视频冻结的默认存活时间。必须覆盖轮询归属粘性会话的
	// 存活期，否则任务仍可轮询时冻结已经被 sweeper 释放，等于白送视频。
	videoBalanceHoldTTL = 2 * time.Hour
	// videoBalanceHoldSweepInterval 是超时冻结的扫描间隔。
	videoBalanceHoldSweepInterval = 2 * time.Minute
	// videoBalanceHoldSweepBatchSize 是单次扫描处理的最大记录数。
	videoBalanceHoldSweepBatchSize = 200
	// videoBalanceHoldCurrency 是媒体工坊展示金额使用的币种。
	videoBalanceHoldCurrency = "USD"
)

// VideoBalanceHoldRequest 描述一次视频生成冻结所需的计费上下文。
type VideoBalanceHoldRequest struct {
	APIKey          *APIKey
	UserID          int64
	GroupID         int64
	AccountID       int64
	Model           string
	Resolution      string
	DurationSeconds int
}

// MultimodalVideoBalanceHoldEnabled 判断当前视频请求是否走两段式余额冻结。
// 订阅制分组按订阅额度结算，简单模式不产生真实扣费，两者都保持原有直扣路径。
func (s *OpenAIGatewayService) MultimodalVideoBalanceHoldEnabled(apiKey *APIKey) bool {
	if s == nil || s.videoBalanceRepository() == nil || apiKey == nil {
		return false
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return false
	}
	if apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		return false
	}
	return true
}

// ReserveMultimodalVideoBalance 在视频任务提交上游前冻结预估金额。
// 冻结金额按与最终结算完全一致的视频定价口径计算：分辨率 × 时长 × 视频倍率。
// 视频单价缺失时直接返回 ErrVideoPriceNotConfigured 拒绝本次生成，不猜测价格。
// 预估金额为 0（例如视频倍率为 0）时仍会建立一条 0 金额的冻结记录：
// 视频生成在创建阶段绝不直接扣费，轮询阶段必须能按上游任务 id 反查记录，
// 否则既无法结算也无法在失败时释放，只能退化成创建期直扣。
func (s *OpenAIGatewayService) ReserveMultimodalVideoBalance(ctx context.Context, req VideoBalanceHoldRequest) (*BalanceHoldRecord, error) {
	repo := s.videoBalanceRepository()
	if repo == nil || req.APIKey == nil || req.APIKey.ID <= 0 || req.UserID <= 0 || strings.TrimSpace(req.Model) == "" {
		return nil, ErrBalanceHoldFailed
	}
	resolution := NormalizeVideoBillingResolutionOrDefault(req.Resolution)
	durationSeconds := NormalizeVideoBillingDurationSecondsOrDefault(req.DurationSeconds)
	multiplier := s.multimodalVideoRateMultiplier(ctx, req.APIKey, req.UserID, req.GroupID)
	cost, err := s.calculateOpenAIVideoCost(ctx, strings.TrimSpace(req.Model), req.APIKey, &OpenAIForwardResult{
		VideoCount: 1, VideoResolution: resolution, VideoDurationSeconds: durationSeconds,
	}, multiplier)
	if err != nil || cost == nil {
		return nil, ErrVideoPriceNotConfigured.WithCause(err)
	}
	now := time.Now()
	record := &BalanceHoldRecord{
		HoldKey: uuid.NewString(), UserID: req.UserID, APIKeyID: req.APIKey.ID,
		GroupID: req.GroupID, AccountID: req.AccountID, Model: strings.TrimSpace(req.Model),
		Resolution: resolution, DurationSeconds: durationSeconds,
		HoldAmount: QuantizeUsageBillingAmount(cost.ActualCost),
		BaseAmount: QuantizeUsageBillingAmount(cost.TotalCost), RateMultiplier: multiplier,
		Currency: videoBalanceHoldCurrency, Status: BalanceHoldStatusPending,
		ExpiresAt: now.Add(videoBalanceHoldTTL), CreatedAt: now, UpdatedAt: now,
	}
	created, err := repo.ReserveVideoBalance(ctx, record)
	if err != nil {
		if errors.Is(err, ErrBalanceHoldInsufficientBalance) {
			return nil, err
		}
		return nil, ErrBalanceHoldFailed.WithCause(err)
	}
	s.invalidateMultimodalVideoBalanceCache(ctx, req.UserID)
	return created, nil
}

// BindMultimodalVideoBalanceUpstream 把本地冻结记录与上游任务 id 关联起来。
// 轮询阶段据此按任务 id 反查冻结金额。绑定失败必须向上暴露：调用方只有确认
// 绑定成功才能认为任务可结算，否则会出现「已受理但无法结算」的冻结记录。
//
// accountID 是最终成功提交任务的账号。跨账号 failover 时冻结是在首次选号账号上
// 创建的，若只更新 Redis 归属缓存，缓存过期或 Redis 故障后轮询/取片会恢复到错误
// 账号，因此必须与 upstream request id 一起写入冻结记录。
func (s *OpenAIGatewayService) BindMultimodalVideoBalanceUpstream(ctx context.Context, record *BalanceHoldRecord, accountID int64, upstreamRequestID string) error {
	if s == nil || s.videoBalanceRepository() == nil || record == nil {
		return ErrBalanceHoldFailed
	}
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	if upstreamRequestID == "" {
		return nil
	}
	if err := s.videoBalanceRepository().BindBalanceHoldUpstreamRequest(ctx, record.HoldKey, upstreamRequestID, accountID); err != nil {
		logger.L().Warn("multimodal_video.bind_upstream_failed",
			zap.String("hold_key", record.HoldKey),
			zap.String("upstream_request_id", upstreamRequestID),
			zap.Int64("account_id", accountID),
			zap.Error(err),
		)
		return ErrBalanceHoldFailed.WithCause(err)
	}
	record.UpstreamRequestID = upstreamRequestID
	if accountID > 0 {
		record.AccountID = accountID
	}
	return nil
}

// ReleaseMultimodalVideoBalance 全额释放一次未结算的冻结。
func (s *OpenAIGatewayService) ReleaseMultimodalVideoBalance(ctx context.Context, record *BalanceHoldRecord) error {
	repo := s.videoBalanceRepository()
	if repo == nil || record == nil {
		return ErrBalanceHoldFailed
	}
	released, err := repo.ReleaseVideoBalance(ctx, record)
	if err != nil {
		return ErrBalanceHoldFailed.WithCause(err)
	}
	if released != nil {
		*record = *released
	}
	s.invalidateMultimodalVideoBalanceCache(ctx, record.UserID)
	return nil
}

// SettleMultimodalVideoBalance commits the capture, ordinary quota effects,
// usage log and terminal state together. The repository returns canonical facts
// on a replay, even if live pricing or a concurrent poll has changed meanwhile.
func (s *OpenAIGatewayService) SettleMultimodalVideoBalance(ctx context.Context, c *gin.Context, account *Account, record *BalanceHoldRecord, actualSeconds int) (*float64, error) {
	repo := s.videoBalanceRepository()
	if repo == nil || record == nil {
		return nil, ErrBalanceHoldFailed
	}
	if record.Status == BalanceHoldStatusSettled && record.ActualAmount != nil {
		return record.ActualAmount, nil
	}
	if record.Status != BalanceHoldStatusPending {
		return nil, ErrBalanceHoldFailed
	}
	capture, err := s.multimodalVideoCapture(ctx, c, account, record, actualSeconds)
	if err != nil {
		return nil, err
	}
	billingCtx, cancel := detachedBillingContext(ctx)
	defer cancel()
	result, err := repo.CaptureVideoBalance(billingCtx, capture)
	if err != nil {
		return nil, ErrBalanceHoldFailed.WithCause(err)
	}
	if result == nil || result.Record == nil || result.Record.Status != BalanceHoldStatusSettled || result.Record.ActualAmount == nil {
		return nil, ErrBalanceHoldFailed
	}
	*record = *result.Record
	s.invalidateMultimodalVideoBalanceCache(billingCtx, record.UserID)
	if s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateAPIKeyRateLimit(billingCtx, record.APIKeyID)
		if result.Billing != nil && result.Billing.Applied && capture.Billing.UserPlatformQuotaCost > 0 {
			s.billingCacheService.IncrementDurableUserPlatformQuotaUsage(record.UserID, capture.Billing.QuotaPlatform, *record.ActualAmount)
		}
	}
	if s.deferredService != nil {
		s.deferredService.ScheduleLastUsedUpdate(record.AccountID)
	}
	return record.ActualAmount, nil
}

// applyMultimodalVideoBilling 在视频响应写回客户端之前完成三件事：
//   - 创建阶段：把冻结记录绑定到上游任务 id，并回显本次冻结金额；
//   - 轮询阶段：按任务状态结算（completed）或全额释放（failed），并回显最终扣费金额；
//   - 仍在处理中：回显当前冻结金额，让客户端知道已占用多少额度。
//
// 返回错误时调用方不得写回响应：绑定或结算失败都必须让客户端重试，而不是把
// 无法计费的任务当成成功交付。
func (s *OpenAIGatewayService) applyMultimodalVideoBilling(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint MultimodalMediaEndpoint,
	requestID string,
	hold *BalanceHoldRecord,
	responseBody []byte,
) ([]byte, error) {
	if s == nil || s.usageBillingRepo == nil {
		return responseBody, nil
	}
	var cost float64
	currency := videoBalanceHoldCurrency
	found := false
	switch {
	case endpoint.IsVideoGenerationRequest():
		if hold == nil {
			return responseBody, nil
		}
		upstreamRequestID := extractGrokMediaVideoRequestID(responseBody)
		if upstreamRequestID == "" {
			// 拿不到上游任务 id 就无法绑定、无法结算，也无法在失败时释放，
			// 只能拒绝交付，避免生成成功的视频被白拿。
			if hold.HoldAmount > 0 {
				return responseBody, fmt.Errorf("multimodal video response is missing upstream request id: hold_key=%s", hold.HoldKey)
			}
		} else if err := s.BindMultimodalVideoBalanceUpstream(ctx, hold, multimodalAccountID(account), upstreamRequestID); err != nil {
			return responseBody, err
		} else if err := s.bindMultimodalVideoRequestOwner(ctx, c, account, hold, upstreamRequestID); err != nil {
			// 归属绑定失败会让后续轮询与取片找不到账号，任务已扣费却无法交付。
			// 必须在写回响应前失败，交由调用方释放冻结并返回错误。
			return responseBody, err
		}
		cost, currency, found = hold.HoldAmount, hold.Currency, true
	case endpoint == MultimodalMediaEndpointVideoStatus:
		record, lookupErr := s.lookupMultimodalVideoBalance(ctx, requestID)
		if lookupErr != nil {
			// 查询失败意味着结算状态未知：不能当作「没有冻结」直接回显响应，
			// 否则客户端会在计费状态未确认的情况下继续取片。
			return responseBody, ErrBalanceHoldFailed.WithCause(lookupErr)
		}
		if record == nil {
			if s.multimodalVideoHoldRequiredForDelivery(c) {
				logger.L().Warn("multimodal_video.status_hold_missing",
					zap.String("upstream_request_id", requestID),
				)
				return responseBody, ErrBalanceHoldFailed.WithCause(fmt.Errorf(
					"multimodal video balance hold is missing for upstream request %s", requestID,
				))
			}
			return responseBody, nil
		}
		if strings.TrimSpace(record.Currency) != "" {
			currency = record.Currency
		}
		found = true
		switch record.Status {
		case BalanceHoldStatusSettled:
			// 重复轮询已完成任务：直接回显已结算金额，不再重复扣费。
			if record.ActualAmount != nil {
				cost = *record.ActualAmount
			} else {
				cost = record.HoldAmount
			}
		case BalanceHoldStatusReleased:
			if s.multimodalVideoHoldRequiredForDelivery(c) {
				return responseBody, ErrBalanceHoldFailed.WithCause(fmt.Errorf(
					"multimodal video balance hold for upstream request %s was released before delivery", requestID,
				))
			}
			cost = 0
		default:
			if record.Status != BalanceHoldStatusPending && s.multimodalVideoHoldRequiredForDelivery(c) {
				return responseBody, ErrBalanceHoldFailed
			}
			status := multimodalVideoResponseStatus(responseBody)
			switch {
			case multimodalVideoStatusCompleted(status):
				settled, settleErr := s.SettleMultimodalVideoBalance(ctx, c, account, record, multimodalVideoResponseSeconds(responseBody))
				if settleErr != nil {
					// 落账失败：记录保持 pending 等待下次轮询重试，响应不显示成功扣费。
					return responseBody, settleErr
				}
				if settled != nil {
					cost = *settled
				} else {
					cost = record.HoldAmount
				}
			case multimodalVideoStatusFailed(status):
				if err := s.ReleaseMultimodalVideoBalance(ctx, record); err != nil {
					logger.L().Warn("multimodal_video.release_failed_task_failed",
						zap.String("hold_key", record.HoldKey),
						zap.Error(err),
					)
				}
				cost = 0
			default:
				cost = record.HoldAmount
			}
		}
	}
	if !found || !gjson.ValidBytes(responseBody) {
		return responseBody, nil
	}
	rendered, err := sjson.SetBytes(responseBody, "cost", cost)
	if err != nil {
		return responseBody, nil
	}
	rendered, err = sjson.SetBytes(rendered, "currency", currency)
	if err != nil {
		return responseBody, nil
	}
	return rendered, nil
}

// multimodalAccountID 安全提取账号 id，account 为 nil 时返回 0（不改变冻结归属）。
func multimodalAccountID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}

// bindMultimodalVideoRequestOwner 在响应写回客户端之前绑定任务归属账号。
// 归属缓存丢失时轮询与取片都会找不到账号，因此绑定失败必须在交付前暴露。
// 没有归属缓存（例如未启用 Redis 的部署）时跳过：轮询仍可用冻结记录里的账号兜底。
func (s *OpenAIGatewayService) bindMultimodalVideoRequestOwner(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	hold *BalanceHoldRecord,
	upstreamRequestID string,
) error {
	if s == nil || s.cache == nil || account == nil || hold == nil {
		return nil
	}
	userID := hold.UserID
	apiKeyID := hold.APIKeyID
	var groupID *int64
	if hold.GroupID > 0 {
		groupID = &hold.GroupID
	}
	if apiKey := getAPIKeyFromContext(c); apiKey != nil {
		if apiKey.UserID > 0 {
			userID = apiKey.UserID
		}
		if apiKey.ID > 0 {
			apiKeyID = apiKey.ID
		}
		groupID = apiKey.GroupID
	}
	if userID <= 0 || apiKeyID <= 0 {
		return nil
	}
	if err := s.BindMultimodalMediaVideoRequestAccount(ctx, groupID, upstreamRequestID, userID, apiKeyID, account.ID); err != nil {
		logger.L().Warn("multimodal_video.bind_request_owner_failed",
			zap.String("hold_key", hold.HoldKey),
			zap.String("upstream_request_id", upstreamRequestID),
			zap.Int64("account_id", account.ID),
			zap.Error(err),
		)
		return ErrBalanceHoldFailed.WithCause(err)
	}
	return nil
}

// settleMultimodalVideoContentBalance 在视频内容真正交付给客户端之前结算冻结。
// 只调 /content 取片、从不轮询 status 的客户端必须同样被扣费。
//
// 冻结计费模式下，查询失败和确认无记录都不允许交付内容：前者无法证明已经结算，
// 后者说明这不是一条合法的无 hold 请求。只有能从认证上下文明确判断该请求不需要
// 冻结（简单模式、订阅制分组）时，缺少冻结记录才按正常路径放行。
func (s *OpenAIGatewayService) settleMultimodalVideoContentBalance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamRequestID string,
) error {
	record, lookupErr := s.lookupMultimodalVideoBalance(ctx, upstreamRequestID)
	if lookupErr != nil {
		if s.multimodalVideoHoldRequiredForDelivery(c) {
			return ErrBalanceHoldFailed.WithCause(lookupErr)
		}
		logger.L().Warn("multimodal_video.content_settle_lookup_failed_without_hold",
			zap.String("upstream_request_id", upstreamRequestID),
			zap.Error(lookupErr),
		)
		return nil
	}
	if record == nil {
		if s.multimodalVideoHoldRequiredForDelivery(c) {
			return ErrBalanceHoldFailed.WithCause(fmt.Errorf(
				"multimodal video balance hold is missing for upstream request %s", upstreamRequestID,
			))
		}
		return nil
	}
	switch record.Status {
	case BalanceHoldStatusSettled:
		// 已结算：资金已经落账，重复取片直接交付，不重复扣费。
		return nil
	case BalanceHoldStatusReleased:
		// 已释放：冻结金额已经退回用户，账号归属也随之失效。此时交付内容等于白送，
		// 必须与状态查询口径一致地拒绝；只有明确不需要冻结的计费模式才放行。
		if s.multimodalVideoHoldRequiredForDelivery(c) {
			logger.L().Warn("multimodal_video.content_delivery_released_hold",
				zap.String("hold_key", record.HoldKey),
				zap.String("upstream_request_id", upstreamRequestID),
			)
			return ErrBalanceHoldFailed.WithCause(fmt.Errorf(
				"multimodal video balance hold for upstream request %s was released before delivery", upstreamRequestID,
			))
		}
		return nil
	}
	// 取片链路拿不到上游返回的实际秒数，按冻结时记录的请求时长结算。
	_, err := s.SettleMultimodalVideoBalance(ctx, c, account, record, record.DurationSeconds)
	return err
}

// multimodalVideoHoldRequiredForDelivery 判断当前请求是否必须依赖冻结记录才能交付。
// 只有能从认证上下文明确确认不需要冻结时才返回 false；无法判定时按需要冻结处理，
// 避免把「状态未知」降级成免费放行。
func (s *OpenAIGatewayService) multimodalVideoHoldRequiredForDelivery(c *gin.Context) bool {
	if s == nil || s.usageBillingRepo == nil {
		return false
	}
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil {
		return true
	}
	return s.MultimodalVideoBalanceHoldEnabled(apiKey)
}

// ResolveMultimodalVideoBalanceAccountID 用冻结记录里保存的账号恢复轮询归属。
// 归属缓存可能因 Redis 故障或 TTL 过期丢失，但已扣费的任务必须仍可轮询与取片。
// 第二个返回值区分「查询失败」与「确认无可用记录」：前者必须返回错误，
// 不能与任务不存在混为一谈。
func (s *OpenAIGatewayService) ResolveMultimodalVideoBalanceAccountID(
	ctx context.Context,
	upstreamRequestID string,
	userID, apiKeyID int64,
) (int64, error) {
	record, err := s.lookupMultimodalVideoBalance(ctx, upstreamRequestID)
	if err != nil {
		return 0, err
	}
	if record == nil {
		return 0, nil
	}
	if record.UserID != userID || record.APIKeyID != apiKeyID {
		return 0, nil
	}
	if record.Status != BalanceHoldStatusPending && record.Status != BalanceHoldStatusSettled {
		return 0, nil
	}
	return record.AccountID, nil
}

// lookupMultimodalVideoBalance 按上游任务 id 反查冻结记录。
// (nil, nil) 表示确认没有记录；非 nil error 表示状态未知，调用方必须 fail-closed，
// 不得把它当成「无冻结」继续交付。
func (s *OpenAIGatewayService) lookupMultimodalVideoBalance(ctx context.Context, upstreamRequestID string) (*BalanceHoldRecord, error) {
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	if s == nil || s.videoBalanceRepository() == nil || upstreamRequestID == "" {
		return nil, nil
	}
	record, err := s.videoBalanceRepository().GetBalanceHoldByUpstreamRequestID(ctx, upstreamRequestID)
	if err != nil {
		logger.L().Warn("multimodal_video.hold_lookup_failed",
			zap.String("upstream_request_id", upstreamRequestID),
			zap.Error(err),
		)
		return nil, err
	}
	return record, nil
}

// ValidateMultimodalVideoPricing 在提交上游前校验视频单价可解析。
// 订阅制分组和简单模式不冻结余额、也不在创建期直扣，必须在这里拦截无价请求，
// 否则要等视频生成完成后才会发现无法计费。
func (s *OpenAIGatewayService) ValidateMultimodalVideoPricing(ctx context.Context, req VideoBalanceHoldRequest) error {
	if s == nil || s.billingService == nil || req.APIKey == nil {
		return nil
	}
	// 简单模式只记录用量、不扣费，不因缺少价格拒绝生成。
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	_, err := s.multimodalVideoHoldAmount(
		ctx,
		req,
		NormalizeVideoBillingResolutionOrDefault(req.Resolution),
		NormalizeVideoBillingDurationSecondsOrDefault(req.DurationSeconds),
	)
	if err != nil {
		return ErrVideoPriceNotConfigured.WithCause(err)
	}
	return nil
}

// multimodalVideoHoldAmount 复用与 RecordUsage 完全一致的视频定价链路计算冻结金额。
// 定价缺失时返回错误，由调用方拒绝本次生成。
func (s *OpenAIGatewayService) multimodalVideoHoldAmount(
	ctx context.Context,
	req VideoBalanceHoldRequest,
	resolution string,
	durationSeconds int,
) (float64, error) {
	if s == nil || s.billingService == nil || req.APIKey == nil {
		return 0, nil
	}
	multiplier := s.multimodalVideoRateMultiplier(ctx, req.APIKey, req.UserID, req.GroupID)
	cost, err := s.calculateOpenAIVideoCost(ctx, strings.TrimSpace(req.Model), req.APIKey, &OpenAIForwardResult{
		VideoCount:           1,
		VideoResolution:      resolution,
		VideoDurationSeconds: durationSeconds,
	}, multiplier)
	if err != nil {
		return 0, err
	}
	if cost == nil {
		return 0, nil
	}
	return cost.ActualCost, nil
}

func (s *OpenAIGatewayService) multimodalVideoRateMultiplier(
	ctx context.Context,
	apiKey *APIKey,
	userID int64,
	groupID int64,
) float64 {
	baseMultiplier := 1.0
	if s != nil && s.cfg != nil {
		baseMultiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey != nil && apiKey.Group != nil && groupID > 0 {
		baseMultiplier = s.ResolveUserGroupRateMultiplier(ctx, userID, groupID, apiKey.Group.RateMultiplier)
	}
	return resolveVideoRateMultiplier(apiKey, baseMultiplier)
}

func (s *OpenAIGatewayService) invalidateMultimodalVideoBalanceCache(ctx context.Context, userID int64) {
	if s == nil || s.billingCacheService == nil || userID <= 0 {
		return
	}
	if err := s.billingCacheService.InvalidateUserBalance(ctx, userID); err != nil {
		logger.L().Debug("multimodal_video.invalidate_balance_cache_failed",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
	}
}

func optionalSessionID(sessionID string) *string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	return &sessionID
}

// StartVideoBalanceHoldSweeper 启动超时冻结扫描。
func (s *OpenAIGatewayService) StartVideoBalanceHoldSweeper(ctx context.Context) {
	if s == nil || s.videoBalanceRepository() == nil {
		return
	}
	s.videoHoldSweepOnce.Do(func() {
		runCtx, cancel := context.WithCancel(ctx)
		s.videoHoldSweepCancel = cancel
		go func() {
			ticker := time.NewTicker(videoBalanceHoldSweepInterval)
			defer ticker.Stop()
			for {
				s.sweepExpiredVideoBalanceHolds(runCtx)
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (s *OpenAIGatewayService) sweepExpiredVideoBalanceHolds(ctx context.Context) {
	if s == nil || s.videoBalanceRepository() == nil {
		return
	}
	now := time.Now()
	records, err := s.videoBalanceRepository().ListExpiredBalanceHolds(ctx, now, videoBalanceHoldSweepBatchSize)
	if err != nil {
		logger.L().Warn("multimodal_video.hold_sweep_list_failed", zap.Error(err))
		return
	}
	for _, record := range records {
		if record == nil {
			continue
		}
		// A bound task already consumed upstream work. Age alone cannot prove
		// failure; only an owner status request confirming failure may refund it.
		// The SQL query filters these before LIMIT; retain this defensive check.
		if strings.TrimSpace(record.UpstreamRequestID) != "" {
			continue
		}
		if err := s.ReleaseMultimodalVideoBalance(ctx, record); err != nil {
			logger.L().Warn("multimodal_video.hold_sweep_release_failed",
				zap.String("hold_key", record.HoldKey),
				zap.Int64("user_id", record.UserID),
				zap.Error(err),
			)
		}
	}
}

// multimodalVideoResponseStatus 读取上游视频任务状态。
func multimodalVideoResponseStatus(responseBody []byte) string {
	if !gjson.ValidBytes(responseBody) {
		return ""
	}
	root := gjson.ParseBytes(responseBody)
	if !root.IsObject() {
		return ""
	}
	status := strings.TrimSpace(root.Get("status").String())
	if status == "" {
		status = strings.TrimSpace(root.Get("data.status").String())
	}
	return status
}

func multimodalVideoStatusCompleted(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "completed", "succeeded", "success":
		return true
	default:
		return false
	}
}

func multimodalVideoStatusFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "cancelled", "canceled", "expired":
		return true
	default:
		return false
	}
}

// multimodalVideoResponseSeconds 读取上游返回的实际视频秒数。
func multimodalVideoResponseSeconds(responseBody []byte) int {
	if !gjson.ValidBytes(responseBody) {
		return 0
	}
	root := gjson.ParseBytes(responseBody)
	if !root.IsObject() {
		return 0
	}
	for _, path := range []string{"seconds", "duration", "data.seconds", "data.duration", "video.seconds", "video.duration"} {
		if value := root.Get(path); value.Exists() && value.Int() > 0 {
			return int(value.Int())
		}
	}
	return 0
}
