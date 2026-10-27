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
	// videoBalanceHoldBoundSweepGrace 是已绑定上游任务的冻结在过期后的额外保留时间。
	// 已绑定意味着任务真实提交到了上游，无条件释放等于白送视频；只有超过宽限期
	// 仍未结算（例如持续落账失败）才回退为释放，避免余额被永久占用。
	videoBalanceHoldBoundSweepGrace = 24 * time.Hour
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
	if s == nil || s.usageBillingRepo == nil || apiKey == nil {
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
	if s == nil || s.usageBillingRepo == nil {
		return nil, nil
	}
	if req.APIKey == nil || req.APIKey.ID <= 0 || req.UserID <= 0 {
		return nil, nil
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil, nil
	}
	resolution := NormalizeVideoBillingResolutionOrDefault(req.Resolution)
	durationSeconds := NormalizeVideoBillingDurationSecondsOrDefault(req.DurationSeconds)
	rawHoldAmount, err := s.multimodalVideoHoldAmount(ctx, req, resolution, durationSeconds)
	if err != nil {
		return nil, ErrVideoPriceNotConfigured.WithCause(err)
	}
	holdAmount := QuantizeUsageBillingAmount(rawHoldAmount)
	if holdAmount < 0 {
		holdAmount = 0
	}

	holdKey := uuid.NewString()
	now := time.Now()
	if holdAmount > 0 {
		cmd := &BalanceHoldCommand{
			RequestID:  BalanceHoldRequestID(holdKey),
			APIKeyID:   req.APIKey.ID,
			UserID:     req.UserID,
			HoldKey:    holdKey,
			HoldAmount: holdAmount,
		}
		if _, err := s.usageBillingRepo.ReserveBalance(ctx, cmd); err != nil {
			if errors.Is(err, ErrBalanceHoldInsufficientBalance) {
				return nil, ErrBalanceHoldInsufficientBalance
			}
			return nil, ErrBalanceHoldFailed.WithCause(err)
		}
	}

	record := &BalanceHoldRecord{
		HoldKey:         holdKey,
		UserID:          req.UserID,
		APIKeyID:        req.APIKey.ID,
		GroupID:         req.GroupID,
		AccountID:       req.AccountID,
		Model:           model,
		Resolution:      resolution,
		DurationSeconds: durationSeconds,
		HoldAmount:      holdAmount,
		Currency:        videoBalanceHoldCurrency,
		Status:          BalanceHoldStatusPending,
		ExpiresAt:       now.Add(videoBalanceHoldTTL),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.usageBillingRepo.CreateBalanceHold(ctx, record); err != nil {
		// 冻结已生效但记录写入失败：立刻释放，避免余额被永久占用。
		if holdAmount > 0 {
			if _, releaseErr := s.usageBillingRepo.ReleaseBalance(ctx, &BalanceHoldCommand{
				RequestID:  BalanceReleaseRequestID(holdKey),
				APIKeyID:   req.APIKey.ID,
				UserID:     req.UserID,
				HoldKey:    holdKey,
				HoldAmount: holdAmount,
			}); releaseErr != nil {
				logger.L().Error("multimodal_video.hold_record_failed_release_failed",
					zap.String("hold_key", holdKey),
					zap.Int64("user_id", req.UserID),
					zap.Error(releaseErr),
				)
			}
		}
		return nil, ErrBalanceHoldFailed.WithCause(err)
	}
	s.invalidateMultimodalVideoBalanceCache(ctx, req.UserID)
	return record, nil
}

// BindMultimodalVideoBalanceUpstream 把本地冻结记录与上游任务 id 关联起来。
// 轮询阶段据此按任务 id 反查冻结金额。绑定失败必须向上暴露：调用方只有确认
// 绑定成功才能认为任务可结算，否则会出现「已受理但无法结算」的冻结记录。
func (s *OpenAIGatewayService) BindMultimodalVideoBalanceUpstream(ctx context.Context, record *BalanceHoldRecord, upstreamRequestID string) error {
	if s == nil || s.usageBillingRepo == nil || record == nil {
		return nil
	}
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	if upstreamRequestID == "" {
		return nil
	}
	if err := s.usageBillingRepo.BindBalanceHoldUpstreamRequest(ctx, record.HoldKey, upstreamRequestID); err != nil {
		logger.L().Warn("multimodal_video.bind_upstream_failed",
			zap.String("hold_key", record.HoldKey),
			zap.String("upstream_request_id", upstreamRequestID),
			zap.Error(err),
		)
		return ErrBalanceHoldFailed.WithCause(err)
	}
	record.UpstreamRequestID = upstreamRequestID
	return nil
}

// ReleaseMultimodalVideoBalance 全额释放一次未结算的冻结。
func (s *OpenAIGatewayService) ReleaseMultimodalVideoBalance(ctx context.Context, record *BalanceHoldRecord) error {
	if s == nil || s.usageBillingRepo == nil || record == nil {
		return nil
	}
	// 已结算或已释放的记录再次释放会把其它冻结的资金退给用户，必须按状态拦截。
	if record.Status != BalanceHoldStatusPending {
		return nil
	}
	holdKey := strings.TrimSpace(record.HoldKey)
	if holdKey == "" {
		return nil
	}
	if record.HoldAmount > 0 {
		_, err := s.usageBillingRepo.ReleaseBalance(ctx, &BalanceHoldCommand{
			RequestID:  BalanceReleaseRequestID(holdKey),
			APIKeyID:   record.APIKeyID,
			UserID:     record.UserID,
			HoldKey:    holdKey,
			HoldAmount: record.HoldAmount,
		})
		if err != nil {
			// 同一 release request id 出现指纹冲突说明此前已释放成功（资金已归还），
			// 视为幂等成功，避免超时扫描反复重试同一条记录。
			if errors.Is(err, ErrUsageBillingRequestConflict) {
				logger.L().Warn("multimodal_video.release_fingerprint_conflict_treated_as_released",
					zap.String("hold_key", holdKey),
				)
			} else {
				return ErrBalanceHoldFailed.WithCause(err)
			}
		}
	}
	if err := s.usageBillingRepo.UpdateBalanceHoldStatus(ctx, holdKey, BalanceHoldStatusReleased, nil); err != nil {
		logger.L().Warn("multimodal_video.release_status_update_failed",
			zap.String("hold_key", holdKey),
			zap.Error(err),
		)
	}
	s.invalidateMultimodalVideoBalanceCache(ctx, record.UserID)
	return nil
}

// SettleMultimodalVideoBalance 按上游实际时长结算冻结金额，返回最终扣费金额。
// 实际金额超过冻结金额时按冻结金额封顶：冻结已经完成，不能反向从余额多扣。
// 落账失败必须返回错误并保持 pending：把失败当成「无事发生」会让响应回显冻结额，
// 随后 sweeper 退款，等于白送视频。
func (s *OpenAIGatewayService) SettleMultimodalVideoBalance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	record *BalanceHoldRecord,
	actualSeconds int,
) (*float64, error) {
	if s == nil || s.usageBillingRepo == nil || record == nil {
		return nil, nil
	}
	holdKey := strings.TrimSpace(record.HoldKey)
	if holdKey == "" {
		return nil, nil
	}
	if record.Status != BalanceHoldStatusPending {
		// 并发轮询/取片已经完成结算：直接回显已落账金额，不重复扣费。
		if record.ActualAmount != nil {
			return record.ActualAmount, nil
		}
		return nil, nil
	}
	if actualSeconds <= 0 {
		actualSeconds = record.DurationSeconds
	}
	actualSeconds = NormalizeVideoBillingDurationSecondsOrDefault(actualSeconds)

	actualAmount := QuantizeUsageBillingAmount(s.multimodalVideoActualAmount(ctx, c, record, actualSeconds))
	if actualAmount < 0 {
		actualAmount = 0
	}
	// 仅在真正冻结到资金时才按冻结额封顶。预估金额为 0 的记录（分组未配置视频价
	// 或创建期定价未命中）没有占用余额，不能把实际金额也压成 0，否则成功生成会漏计费。
	if record.HoldAmount > 0 && actualAmount > record.HoldAmount {
		logger.L().Warn("multimodal_video.settlement_cost_exceeds_hold",
			zap.String("hold_key", holdKey),
			zap.Float64("hold_amount", record.HoldAmount),
			zap.Float64("actual_amount", actualAmount),
		)
		actualAmount = record.HoldAmount
	}

	captureResult, err := s.usageBillingRepo.CaptureBalance(ctx, &BalanceHoldCommand{
		RequestID:    BalanceCaptureRequestID(holdKey),
		APIKeyID:     record.APIKeyID,
		UserID:       record.UserID,
		HoldKey:      holdKey,
		HoldAmount:   record.HoldAmount,
		ActualAmount: actualAmount,
	})
	if err != nil {
		logger.L().Warn("multimodal_video.settlement_capture_failed",
			zap.String("hold_key", holdKey),
			zap.Int64("user_id", record.UserID),
			zap.Error(err),
		)
		return nil, ErrBalanceHoldFailed.WithCause(err)
	}
	if captureResult != nil && !captureResult.Applied {
		// 并发轮询已经完成结算：资金已按实际金额落账，不再重复写用量流水。
		if record.ActualAmount != nil {
			return record.ActualAmount, nil
		}
		return nil, nil
	}

	settled := actualAmount
	if err := s.usageBillingRepo.UpdateBalanceHoldStatus(ctx, holdKey, BalanceHoldStatusSettled, &settled); err != nil {
		logger.L().Warn("multimodal_video.settlement_status_update_failed",
			zap.String("hold_key", holdKey),
			zap.Error(err),
		)
	}
	s.invalidateMultimodalVideoBalanceCache(ctx, record.UserID)
	s.recordMultimodalVideoSettlementUsageLog(ctx, c, account, record, actualSeconds, actualAmount)
	return &settled, nil
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
		} else if err := s.BindMultimodalVideoBalanceUpstream(ctx, hold, upstreamRequestID); err != nil {
			return responseBody, err
		} else if err := s.bindMultimodalVideoRequestOwner(ctx, c, account, hold, upstreamRequestID); err != nil {
			// 归属绑定失败会让后续轮询与取片找不到账号，任务已扣费却无法交付。
			// 必须在写回响应前失败，交由调用方释放冻结并返回错误。
			return responseBody, err
		}
		cost, currency, found = hold.HoldAmount, hold.Currency, true
	case endpoint == MultimodalMediaEndpointVideoStatus:
		record := s.lookupMultimodalVideoBalance(ctx, requestID)
		if record == nil {
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
			cost = 0
		default:
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
func (s *OpenAIGatewayService) settleMultimodalVideoContentBalance(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	upstreamRequestID string,
) error {
	record := s.lookupMultimodalVideoBalance(ctx, upstreamRequestID)
	if record == nil {
		return nil
	}
	if record.Status != BalanceHoldStatusPending {
		return nil
	}
	// 取片链路拿不到上游返回的实际秒数，按冻结时记录的请求时长结算。
	_, err := s.SettleMultimodalVideoBalance(ctx, c, account, record, record.DurationSeconds)
	return err
}

// ResolveMultimodalVideoBalanceAccountID 用冻结记录里保存的账号恢复轮询归属。
// 归属缓存可能因 Redis 故障或 TTL 过期丢失，但已扣费的任务必须仍可轮询与取片。
func (s *OpenAIGatewayService) ResolveMultimodalVideoBalanceAccountID(
	ctx context.Context,
	upstreamRequestID string,
	userID, apiKeyID int64,
) int64 {
	record := s.lookupMultimodalVideoBalance(ctx, upstreamRequestID)
	if record == nil {
		return 0
	}
	if record.UserID != userID || record.APIKeyID != apiKeyID {
		return 0
	}
	if record.Status != BalanceHoldStatusPending && record.Status != BalanceHoldStatusSettled {
		return 0
	}
	return record.AccountID
}

func (s *OpenAIGatewayService) lookupMultimodalVideoBalance(ctx context.Context, upstreamRequestID string) *BalanceHoldRecord {
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	if s == nil || s.usageBillingRepo == nil || upstreamRequestID == "" {
		return nil
	}
	record, err := s.usageBillingRepo.GetBalanceHoldByUpstreamRequestID(ctx, upstreamRequestID)
	if err != nil {
		logger.L().Warn("multimodal_video.hold_lookup_failed",
			zap.String("upstream_request_id", upstreamRequestID),
			zap.Error(err),
		)
		return nil
	}
	return record
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

func (s *OpenAIGatewayService) multimodalVideoActualAmount(
	ctx context.Context,
	c *gin.Context,
	record *BalanceHoldRecord,
	actualSeconds int,
) float64 {
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil || apiKey.ID != record.APIKeyID || apiKey.UserID != record.UserID {
		// 缺少可信的计费上下文时按冻结金额结算，保证已经发生的生成仍然被计费。
		return record.HoldAmount
	}
	multiplier := s.multimodalVideoRateMultiplier(ctx, apiKey, record.UserID, record.GroupID)
	cost, err := s.calculateOpenAIVideoCost(ctx, strings.TrimSpace(record.Model), apiKey, &OpenAIForwardResult{
		VideoCount:           1,
		VideoResolution:      record.Resolution,
		VideoDurationSeconds: actualSeconds,
	}, multiplier)
	if err != nil || cost == nil {
		// 冻结阶段已按同一价格口径校验通过；结算阶段定价缺失（例如分组价被删）
		// 时按冻结金额结算，避免已经完成的生成漏计费。
		if err != nil {
			logger.L().Warn("multimodal_video.settlement_price_unavailable",
				zap.String("hold_key", record.HoldKey),
				zap.String("model", record.Model),
				zap.Error(err),
			)
		}
		return record.HoldAmount
	}
	return cost.ActualCost
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

// recordMultimodalVideoSettlementUsageLog 在结算成功后补写 usage_log。
// 视频生成在创建阶段只冻结不写用量，因此这里是一次视频请求唯一的计费流水。
func (s *OpenAIGatewayService) recordMultimodalVideoSettlementUsageLog(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	record *BalanceHoldRecord,
	actualSeconds int,
	actualAmount float64,
) {
	if s == nil || s.usageLogRepo == nil || record == nil {
		return
	}
	billingMode := string(BillingModeVideo)
	resolution := NormalizeVideoBillingResolutionOrDefault(record.Resolution)
	inboundEndpoint := "/v1/videos/generations"
	if c != nil && c.Request != nil && c.Request.URL != nil && strings.TrimSpace(c.Request.URL.Path) != "" {
		inboundEndpoint = strings.TrimSpace(c.Request.URL.Path)
	}
	upstreamEndpoint := strings.TrimSpace(GetActualOpenAIUpstreamEndpoint(c))
	if upstreamEndpoint == "" {
		upstreamEndpoint = "/v1/videos/generations"
	}
	var accountID int64
	accountRateMultiplier := 1.0
	if account != nil {
		accountID = account.ID
		accountRateMultiplier = account.BillingRateMultiplier()
	}
	if accountID <= 0 {
		accountID = record.AccountID
	}
	multiplier := s.multimodalVideoRateMultiplier(ctx, getAPIKeyFromContext(c), record.UserID, record.GroupID)
	sessionID := ExtractClientSessionID(c)
	// 「总耗时」记录视频真实生成时长：从创建阶段冻结建立到轮询结算完成的时间差。
	// 结算发生在轮询请求里，若直接取本次请求耗时只会得到几百毫秒的 HTTP 往返时间。
	durationMs := int(time.Since(record.CreatedAt).Milliseconds())
	if record.CreatedAt.IsZero() || durationMs <= 0 {
		durationMs = actualSeconds * 1000
	}
	usageLog := &UsageLog{
		UserID:                record.UserID,
		APIKeyID:              record.APIKeyID,
		AccountID:             accountID,
		RequestID:             strings.TrimSpace(record.UpstreamRequestID),
		Model:                 record.Model,
		RequestedModel:        record.Model,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		VideoCount:            1,
		VideoResolution:       &resolution,
		VideoDurationSeconds:  &actualSeconds,
		ImageOutputCost:       actualAmount,
		TotalCost:             actualAmount,
		ActualCost:            actualAmount,
		RateMultiplier:        multiplier,
		AccountRateMultiplier: &accountRateMultiplier,
		BillingType:           BillingTypeBalance,
		RequestType:           RequestTypeSync,
		BillingMode:           &billingMode,
		DurationMs:            &durationMs,
		SessionID:             optionalSessionID(sessionID),
		CreatedAt:             time.Now(),
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.multimodal_video_settlement")
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
	if s == nil || s.usageBillingRepo == nil {
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
	if s == nil || s.usageBillingRepo == nil {
		return
	}
	now := time.Now()
	records, err := s.usageBillingRepo.ListExpiredBalanceHolds(ctx, now, videoBalanceHoldSweepBatchSize)
	if err != nil {
		logger.L().Warn("multimodal_video.hold_sweep_list_failed", zap.Error(err))
		return
	}
	for _, record := range records {
		if record == nil {
			continue
		}
		// 已绑定上游任务的冻结不能无条件释放：任务已经真实生成，释放等于白送。
		// 只有超过宽限期仍未结算（例如持续落账失败）才回退为释放，避免永久占用余额。
		if strings.TrimSpace(record.UpstreamRequestID) != "" &&
			now.Before(record.ExpiresAt.Add(videoBalanceHoldBoundSweepGrace)) {
			logger.L().Warn("multimodal_video.hold_sweep_skipped_bound",
				zap.String("hold_key", record.HoldKey),
				zap.String("upstream_request_id", record.UpstreamRequestID),
				zap.Time("expires_at", record.ExpiresAt),
			)
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
