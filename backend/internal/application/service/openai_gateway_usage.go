package service

// 本文件由 openai_gateway_service.go 纯移动拆分而来：用量记录、计费成本计算与
// Codex 用量快照。仅做代码搬迁，无任何行为变更。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/timezone"
	"go.uber.org/zap"
)

// OpenAIRecordUsageInput input for recording usage
type OpenAIRecordUsageInput struct {
	Result             *OpenAIForwardResult
	APIKey             *APIKey
	User               *User
	Account            *Account
	Subscription       *UserSubscription
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string // 请求的 User-Agent
	IPAddress          string // 请求的客户端 IP 地址
	SessionID          string // 客户端显式会话标识（session_id / X-Session-Id 等请求头），仅用于用量行会话关联
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
	QuotaPlatform      string // user×platform quota platform resolved by the handler before async billing.
	PricingAt          time.Time
	// CyberBlocked 为 true 时把该用量行标记为 cyber（request_type=cyber），计费逻辑不变。
	CyberBlocked bool
	ChannelUsageFields
}

// CyberPolicyUsageInput 是 cyber 拒绝、未走正常 RecordUsage 的请求记录用量的入参。
// 用量按上游真实 token 计费，与 WS cyber 及正常请求口径一致（InputTokens/OutputTokens
// 取自上游 response.failed 报告的 usage，即 mark.UpstreamInTok/OutTok）。
type CyberPolicyUsageInput struct {
	ReasoningTokens *int
	APIKey          *APIKey
	Account         *Account
	Subscription    *UserSubscription
	RequestID       string
	Model           string
	Stream          bool
	InputTokens     int
	OutputTokens    int
	// 渠道归因与请求级 meta，使 cyber 计费行与正常 RecordUsage 行口径一致
	// （否则 cyber 行 channel_id 等为空，渠道维度统计会遗漏 cyber 命中）。
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	SessionID          string
	RequestPayloadHash string
	APIKeyService      APIKeyQuotaUpdater
	ChannelUsageFields
}

// RecordCyberPolicyUsageLog 为被上游 cyber_policy 拒绝、未走正常 RecordUsage 的请求
// （HTTP forward 返回错误路径）记录用量并按上游真实 token 计费，使其与 WS cyber 路径、
// 与正常请求的计费口径统一（不再是 tokens=0 免费行）。token 取自上游 response.failed
// 报告的 usage（非流式直接拒通常为 0，cost 随之为 0）。复用 RecordUsage 完成成本计算、
// 扣费与用量行写入（request_type=cyber 由 CyberBlocked 置位）。仅 forward 返回错误的
// 路径由 handler 调用，避免与成功路径的正常 RecordUsage 重复。
func (s *OpenAIGatewayService) RecordCyberPolicyUsageLog(ctx context.Context, in CyberPolicyUsageInput) {
	if s == nil || in.APIKey == nil || in.APIKey.User == nil || in.Account == nil || strings.TrimSpace(in.Model) == "" {
		return
	}
	result := &OpenAIForwardResult{
		RequestID: in.RequestID,
		Model:     in.Model,
		Stream:    in.Stream,
		Usage: OpenAIUsage{
			InputTokens:     in.InputTokens,
			OutputTokens:    in.OutputTokens,
			ReasoningTokens: in.ReasoningTokens,
		},
	}
	if err := s.RecordUsage(ctx, &OpenAIRecordUsageInput{
		Result:             result,
		APIKey:             in.APIKey,
		User:               in.APIKey.User,
		Account:            in.Account,
		Subscription:       in.Subscription,
		InboundEndpoint:    in.InboundEndpoint,
		UpstreamEndpoint:   in.UpstreamEndpoint,
		UserAgent:          in.UserAgent,
		IPAddress:          in.IPAddress,
		SessionID:          in.SessionID,
		RequestPayloadHash: in.RequestPayloadHash,
		APIKeyService:      in.APIKeyService,
		ChannelUsageFields: in.ChannelUsageFields,
		CyberBlocked:       true,
	}); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber usage record failed: request_id=%s err=%v", in.RequestID, err)
	}
}

// ResolveUserGroupRateMultiplier resolves the same cached multiplier used by OpenAI usage billing.
func (s *OpenAIGatewayService) ResolveUserGroupRateMultiplier(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if s == nil {
		return groupDefaultMultiplier
	}
	resolver := s.userGroupRateResolver
	if resolver == nil {
		resolver = newUserGroupRateResolver(nil, nil, resolveUserGroupRateCacheTTL(s.cfg), nil, "service.openai_gateway")
	}
	return resolver.Resolve(ctx, userID, groupID, groupDefaultMultiplier)
}

func openAIUsagePricingAt(input *OpenAIRecordUsageInput) time.Time {
	if input != nil && !input.PricingAt.IsZero() {
		return input.PricingAt
	}
	return timezone.Now()
}

// RecordUsage records usage and deducts balance
func (s *OpenAIGatewayService) RecordUsage(ctx context.Context, input *OpenAIRecordUsageInput) error {
	if input == nil {
		return errors.New("openai usage input is nil")
	}
	result := input.Result
	if result == nil {
		return errors.New("openai usage result is nil")
	}
	if s.rateLimitService != nil && input.Account != nil && input.Account.Platform == PlatformOpenAI {
		s.rateLimitService.ResetOpenAI403Counter(ctx, input.Account.ID)
	}

	apiKey := input.APIKey
	user := input.User
	account := input.Account
	subscription := input.Subscription
	if !isVideoUsageResult(result) && !result.ImageBillingSizeLocked {
		ApplyOpenAIImageBillingResolution(result)
	}

	// OpenAI input_tokens 是总输入，包含缓存读取和缓存写入明细。
	// 将三类 token 拆成互斥桶，避免缓存写入同时按普通输入和 cache_write 重复计费。
	actualInputTokens := result.Usage.InputTokens - result.Usage.CacheReadInputTokens - result.Usage.CacheCreationInputTokens
	if actualInputTokens < 0 {
		actualInputTokens = 0
	}

	// Calculate cost
	tokens := UsageTokens{
		InputTokens:         actualInputTokens,
		ImageInputTokens:    result.Usage.ImageInputTokens,
		OutputTokens:        result.Usage.OutputTokens,
		CacheCreationTokens: result.Usage.CacheCreationInputTokens,
		CacheReadTokens:     result.Usage.CacheReadInputTokens,
		ImageOutputTokens:   result.Usage.ImageOutputTokens,
	}

	// Get rate multiplier
	multiplier := 1.0
	if s.cfg != nil {
		multiplier = s.cfg.Default.RateMultiplier
	}
	if apiKey.GroupID != nil && apiKey.Group != nil {
		multiplier = s.ResolveUserGroupRateMultiplier(ctx, user.ID, *apiKey.GroupID, apiKey.Group.RateMultiplier)
	}
	// 默认 token 倍率叠加高峰因子；开启独立生图倍率时，图片 token 与按次图片改用独立倍率。高峰因子按请求时刻现算，
	// 不并入上面的 Resolve，以免污染 user:group 倍率缓存。
	baseMultiplier := multiplier
	multiplier, imageMultiplier := computePeakAwareMultipliers(apiKey, baseMultiplier, openAIUsagePricingAt(input))
	videoMultiplier := resolveVideoRateMultiplier(apiKey, baseMultiplier)

	var cost *CostBreakdown
	var costResolution openAIRecordUsageCostResolution
	var err error
	billingModel := forwardResultBillingModel(result.Model, result.UpstreamModel)
	if result.BillingModel != "" {
		billingModel = strings.TrimSpace(result.BillingModel)
	}
	if input.BillingModelSource == BillingModelSourceChannelMapped && input.ChannelMappedModel != "" && input.ChannelMappedModel != input.OriginalModel {
		billingModel = input.ChannelMappedModel
	}
	if input.BillingModelSource == BillingModelSourceRequested && input.OriginalModel != "" {
		billingModel = input.OriginalModel
	}
	billingModels := usageBillingModelCandidates(
		billingModel,
		result.BillingModel,
		input.ChannelMappedModel,
		input.OriginalModel,
		result.UpstreamModel,
		result.Model,
	)
	isVideoUsage := isVideoUsageResult(result)
	billingServiceTier := result.ServiceTier
	if result.Usage.ServiceTier != nil {
		billingServiceTier = result.Usage.ServiceTier
	}
	serviceTier := ""
	if billingServiceTier != nil {
		serviceTier = strings.TrimSpace(*billingServiceTier)
	}
	billingAccount := account
	if account.IsShadow() {
		billingAccount, err = resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return err
		}
	}
	longContextBillingEnabled := accountLongContextBillingEnabled(billingAccount)
	modelLongContextExempt := openAISubscriptionLongContextExempt(billingAccount, result)
	costResolution, err = s.calculateOpenAIRecordUsageCostWithResolution(
		ctx,
		result,
		apiKey,
		billingModels,
		multiplier,
		imageMultiplier,
		videoMultiplier,
		baseMultiplier,
		tokens,
		serviceTier,
		longContextBillingEnabled,
		modelLongContextExempt,
	)
	if costResolution.Cost != nil {
		cost = costResolution.Cost
	}
	if err != nil {
		if !isUsagePricingUnavailableError(err) {
			return err
		}
		logger.L().With(
			zap.String("component", "service.openai_gateway"),
			zap.Strings("billing_models", billingModels),
			zap.String("requested_model", input.OriginalModel),
			zap.String("mapped_model", input.ChannelMappedModel),
			zap.String("upstream_model", result.UpstreamModel),
			zap.Int64("api_key_id", apiKey.ID),
			zap.Int64("account_id", account.ID),
		).Warn("openai_usage.pricing_missing_record_zero_cost", zap.Error(err))
		cost = &CostBreakdown{BillingMode: string(BillingModeToken)}
	}

	// Use the actual successful baseline candidate and its already-resolved
	// pricing source. The first candidate can be unpriced while a later candidate
	// carries an administrator-defined channel price, so first-candidate inference
	// could otherwise bypass that price.
	if costResolution.Cost != nil {
		if responseModel := responseModelBillingDeclaration(
			input.BillingModelSource,
			result.UpstreamResponseModel,
			result.UpstreamResponseModelConflict,
			result.ImageCount > 0 || result.VideoCount > 0 || result.WebSearchCalls > 0,
		); responseModel != "" && !strings.EqualFold(responseModel, strings.TrimSpace(costResolution.BillingModel)) {
			responsePricing := s.resolveOpenAIBillingPricing(ctx, responseModel, apiKey)
			if responseModelPricingIdentified(responsePricing, s.billingService, responseModel) {
				responseCost, usedResponsePricing, responseErr := s.calculateOpenAIRecordUsageTokenCost(
					ctx,
					apiKey,
					responseModel,
					multiplier,
					nil,
					tokens,
					serviceTier,
					longContextBillingEnabled,
					modelLongContextExempt,
					responsePricing,
				)
				if responseErr == nil && responseModelBillingAdoptable(
					cost,
					responseCost,
					pricingResolvedFromChannel(costResolution.Pricing),
					pricingResolvedFromChannel(usedResponsePricing),
				) {
					logResponseModelBillingApplied(
						"service.openai_gateway", account, result.RequestID,
						costResolution.BillingModel, responseModel, cost, responseCost,
					)
					cost = responseCost
				}
			}
		}
	}

	// Determine billing type
	isSubscriptionBilling := subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
	billingType := BillingTypeBalance
	if isSubscriptionBilling {
		billingType = BillingTypeSubscription
	}

	// Create usage log
	durationMs := int(result.Duration.Milliseconds())
	accountRateMultiplier := account.BillingRateMultiplier()
	requestID := resolveUsageBillingRequestID(ctx, result.RequestID)

	// 确定 RequestedModel（渠道映射前的原始模型）
	requestedModel := result.Model
	if input.OriginalModel != "" {
		requestedModel = input.OriginalModel
	}
	sentModel := upstreamSentModel(result.Model, result.UpstreamModel)
	if result.UpstreamResponseModelConflict {
		logger.L().Warn("upstream_response_model_conflict",
			zap.String("platform", account.Platform),
			zap.Int64("account_id", account.ID),
			zap.String("request_id", requestID),
			zap.String("sent_model", sentModel),
			zap.String("selected_response_model", strings.TrimSpace(result.UpstreamResponseModel)),
		)
	}

	usageLog := &UsageLog{
		ReasoningTokens:       result.Usage.ReasoningTokens,
		UserID:                user.ID,
		APIKeyID:              apiKey.ID,
		AccountID:             account.ID,
		RequestID:             requestID,
		Model:                 result.Model,
		RequestedModel:        requestedModel,
		UpstreamModel:         optionalUsageUpstreamModel(result.UpstreamModel, result.Model, requestedModel),
		UpstreamResponseModel: optionalTrimmedStringPtr(result.UpstreamResponseModel),
		UpstreamModelMismatch: upstreamModelMismatch(sentModel, result.UpstreamResponseModel),
		ServiceTier:           billingServiceTier,
		ReasoningEffort:       result.ReasoningEffort,
		InboundEndpoint:       optionalTrimmedStringPtr(input.InboundEndpoint),
		UpstreamEndpoint:      optionalTrimmedStringPtr(input.UpstreamEndpoint),
		InputTokens:           actualInputTokens,
		OutputTokens:          result.Usage.OutputTokens,
		CacheCreationTokens:   result.Usage.CacheCreationInputTokens,
		CacheReadTokens:       result.Usage.CacheReadInputTokens,
		ImageInputTokens:      result.Usage.ImageInputTokens,
		ImageOutputTokens:     result.Usage.ImageOutputTokens,
		ImageCount:            result.ImageCount,
		ImageSize:             optionalTrimmedStringPtr(result.ImageSize),
		ImageInputSize:        optionalTrimmedStringPtr(result.ImageInputSize),
		ImageOutputSize:       optionalTrimmedStringPtr(result.ImageOutputSize),
		ImageSizeSource:       optionalTrimmedStringPtr(result.ImageSizeSource),
		ImageSizeBreakdown:    result.ImageSizeBreakdown,
	}
	if isVideoUsage {
		usageLog.VideoCount = result.VideoCount
		usageLog.VideoResolution = optionalTrimmedStringPtr(NormalizeVideoBillingResolutionOrDefault(result.VideoResolution))
		videoDurationSeconds := NormalizeVideoBillingDurationSecondsOrDefault(result.VideoDurationSeconds)
		usageLog.VideoDurationSeconds = &videoDurationSeconds
	}
	if cost != nil {
		usageLog.InputCost = cost.InputCost
		usageLog.ImageInputCost = cost.ImageInputCost
		usageLog.OutputCost = cost.OutputCost
		usageLog.ImageOutputCost = cost.ImageOutputCost
		usageLog.CacheCreationCost = cost.CacheCreationCost
		usageLog.CacheReadCost = cost.CacheReadCost
		usageLog.TotalCost = cost.TotalCost
		usageLog.ActualCost = cost.ActualCost
		usageLog.LongContextBillingApplied = cost.LongContextBillingApplied
	}
	if isVideoUsage && (cost == nil || cost.BillingMode != string(BillingModeToken)) {
		usageLog.RateMultiplier = videoMultiplier
	} else if result.ImageCount > 0 && (cost == nil || cost.BillingMode != string(BillingModeToken)) {
		usageLog.RateMultiplier = imageMultiplier
	} else if !isVideoUsage && result.ImageCount > 0 && cost != nil && cost.BillingMode == string(BillingModeToken) {
		if tokenImageMultiplier := tokenImageRateMultiplier(apiKey, imageMultiplier); tokenImageMultiplier != nil {
			usageLog.RateMultiplier = *tokenImageMultiplier
		} else {
			usageLog.RateMultiplier = multiplier
		}
	} else {
		usageLog.RateMultiplier = multiplier
	}
	usageLog.AccountRateMultiplier = &accountRateMultiplier
	usageLog.BillingType = billingType
	usageLog.Stream = result.Stream
	if input.CyberBlocked {
		usageLog.RequestType = RequestTypeCyberBlocked
	}
	usageLog.OpenAIWSMode = result.OpenAIWSMode
	usageLog.DurationMs = &durationMs
	usageLog.FirstTokenMs = usageFirstTokenMs(result.FirstTokenMs, result.ImageCount)
	usageLog.OpenAITiming = result.OpenAITiming
	usageLog.CreatedAt = time.Now()
	// 设置渠道信息
	usageLog.ChannelID = optionalInt64Ptr(input.ChannelID)
	usageLog.ModelMappingChain = optionalTrimmedStringPtr(input.ModelMappingChain)
	// 设置计费模式
	if cost != nil && cost.BillingMode != "" {
		billingMode := cost.BillingMode
		usageLog.BillingMode = &billingMode
	} else if isVideoUsage {
		billingMode := string(BillingModeVideo)
		usageLog.BillingMode = &billingMode
	} else if result.ImageCount > 0 {
		billingMode := string(BillingModeImage)
		usageLog.BillingMode = &billingMode
	} else {
		billingMode := string(BillingModeToken)
		usageLog.BillingMode = &billingMode
	}
	// 添加 UserAgent
	if input.UserAgent != "" {
		usageLog.UserAgent = &input.UserAgent
	}

	// 添加 IPAddress
	if input.IPAddress != "" {
		usageLog.IPAddress = &input.IPAddress
	}

	// 添加 SessionID（客户端显式会话标识；缺失/无效时保持 nil）
	usageLog.SessionID = optionalTrimmedStringPtr(input.SessionID)

	if apiKey.GroupID != nil {
		usageLog.GroupID = apiKey.GroupID
	}
	if subscription != nil {
		usageLog.SubscriptionID = &subscription.ID
	}

	// 计算账号统计定价费用（使用最终上游模型匹配自定义规则）
	if apiKey.GroupID != nil {
		applyAccountStatsCost(ctx, usageLog, s.channelService, s.billingService,
			account.ID, *apiKey.GroupID, result.UpstreamModel, result.Model,
			tokens, cost.TotalCost, longContextBillingEnabled && !modelLongContextExempt,
		)
	}

	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.openai_gateway")
		logger.LegacyPrintf("service.openai_gateway", "[SIMPLE MODE] Usage recorded (not billed): user=%d, tokens=%d", usageLog.UserID, usageLog.TotalTokens())
		s.deferredService.ScheduleLastUsedUpdate(account.ID)
		return nil
	}

	// Async usage billing runs outside the original request context, so it
	// cannot recover ForcePlatform there. Fall back for internal/test callers.
	quotaPlatform := input.QuotaPlatform
	if quotaPlatform == "" {
		quotaPlatform = PlatformFromAPIKey(apiKey)
	}

	billingErr := func() error {
		_, err := applyUsageBilling(ctx, requestID, usageLog, &postUsageBillingParams{
			Cost:                  cost,
			AccountStatsCost:      usageLog.AccountStatsCost,
			User:                  user,
			APIKey:                apiKey,
			Account:               account,
			Subscription:          subscription,
			RequestPayloadHash:    resolveUsageBillingPayloadFingerprint(ctx, input.RequestPayloadHash),
			IsSubscriptionBill:    isSubscriptionBilling,
			AccountRateMultiplier: accountRateMultiplier,
			APIKeyService:         input.APIKeyService,
			Platform:              quotaPlatform,
		}, s.billingDeps(), s.usageBillingRepo)
		return err
	}()

	if billingErr != nil {
		usageLog.ActualCost = 0
		writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.openai_gateway")
		return billingErr
	}
	writeUsageLogBestEffort(ctx, s.usageLogRepo, usageLog, "service.openai_gateway")

	return nil
}

type openAIRecordUsageCostResolution struct {
	Cost         *CostBreakdown
	BillingModel string
	Pricing      *ResolvedPricing
}

func (s *OpenAIGatewayService) calculateOpenAIRecordUsageCostWithResolution(
	ctx context.Context,
	result *OpenAIForwardResult,
	apiKey *APIKey,
	billingModels []string,
	multiplier float64,
	imageMultiplier float64,
	videoMultiplier float64,
	webSearchMultiplier float64,
	tokens UsageTokens,
	serviceTier string,
	longContextBillingEnabled bool,
	modelLongContextExempt bool,
) (openAIRecordUsageCostResolution, error) {
	billingModel := firstUsageBillingModel(billingModels)
	if result != nil && result.WebSearchCalls > 0 {
		// Codex alpha/search 网页搜索按次计费：上游不返回 usage/token 字段，单价只取
		// 分组覆盖价（nil 时默认 0.01 = 官方 $10/1000 次），不参与渠道级模型定价。
		// 倍率与 image/video 按次口径一致：使用不含高峰因子的基础倍率
		//（用户专属 > 分组 rate_multiplier > 系统默认），与分组表单的价格预览承诺一致。
		return openAIRecordUsageCostResolution{
			Cost:         s.billingService.CalculateWebSearchCost(result.WebSearchCalls, webSearchPricePerCallFromAPIKey(apiKey), webSearchMultiplier),
			BillingModel: billingModel,
		}, nil
	}
	if isVideoUsageResult(result) {
		if resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey); resolved == nil || resolved.Mode != BillingModeToken {
			cost, err := s.calculateOpenAIVideoCost(ctx, billingModel, apiKey, result, videoMultiplier)
			if err != nil {
				return openAIRecordUsageCostResolution{}, fmt.Errorf("calculate video cost for model %s: %w", billingModel, err)
			}
			return openAIRecordUsageCostResolution{
				Cost:         cost,
				BillingModel: billingModel,
			}, nil
		}
	}
	if result != nil && result.ImageCount > 0 {
		// 渠道定价为 token 计费时走 token 路径，否则走图片计费
		if resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey); imageCountBillingApplies(apiKey, result.ImageEndpoint) || resolved == nil || resolved.Mode != BillingModeToken {
			cost, err := s.calculateOpenAIImageCost(ctx, billingModel, apiKey, result, imageMultiplier)
			if err != nil {
				return openAIRecordUsageCostResolution{}, fmt.Errorf("calculate image cost for model %s: %w", billingModel, err)
			}
			return openAIRecordUsageCostResolution{
				Cost:         cost,
				BillingModel: billingModel,
			}, nil
		}
	}
	if len(billingModels) == 0 || billingModel == "" {
		return openAIRecordUsageCostResolution{}, errors.New("openai usage billing model is empty")
	}
	var lastErr error
	var tokenImageMultiplier *float64
	if result != nil && result.ImageCount > 0 && !isVideoUsageResult(result) {
		tokenImageMultiplier = tokenImageRateMultiplier(apiKey, imageMultiplier)
	}
	for _, candidate := range billingModels {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		cost, resolved, err := s.calculateOpenAIRecordUsageTokenCost(
			ctx,
			apiKey,
			candidate,
			multiplier,
			tokenImageMultiplier,
			tokens,
			serviceTier,
			longContextBillingEnabled,
			modelLongContextExempt,
			nil,
		)
		if err == nil {
			return openAIRecordUsageCostResolution{
				Cost:         cost,
				BillingModel: candidate,
				Pricing:      resolved,
			}, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no non-empty billing model candidates")
	}
	return openAIRecordUsageCostResolution{}, fmt.Errorf("calculate OpenAI usage cost failed for billing models %s: %w", strings.Join(billingModels, ","), lastErr)
}

// isVideoUsageResult reports whether the forward result describes a video
// generation. Video handlers are the only producers of VideoCount, so the
// counter alone is authoritative. Gating on model names would misclassify
// non-Grok video models (multimodal seedance/dreamina) as image usage.
func isVideoUsageResult(result *OpenAIForwardResult) bool {
	return result != nil && result.VideoCount > 0
}

func isUsagePricingUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrModelPricingUnavailable) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no pricing available") || strings.Contains(msg, "pricing not found")
}

// The account-level switch belongs to OpenAI accounts. Other platforms, most
// notably Grok, are governed by the group switch and their model price card.
func accountLongContextBillingEnabled(account *Account) bool {
	return account == nil || !account.IsOpenAI() || account.IsOpenAILongContextBillingEnabled()
}

func (s *OpenAIGatewayService) calculateOpenAIRecordUsageTokenCost(
	ctx context.Context,
	apiKey *APIKey,
	billingModel string,
	multiplier float64,
	imageMultiplier *float64,
	tokens UsageTokens,
	serviceTier string,
	longContextBillingEnabled bool,
	modelLongContextExempt bool,
	resolved *ResolvedPricing,
) (*CostBreakdown, *ResolvedPricing, error) {
	if s.resolver != nil && apiKey.Group != nil {
		gid := apiKey.Group.ID
		if resolved == nil {
			resolved = s.resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: &gid, Group: apiKey.Group})
		}
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:                            ctx,
			Model:                          billingModel,
			GroupID:                        &gid,
			Group:                          apiKey.Group,
			Tokens:                         tokens,
			RequestCount:                   1,
			RateMultiplier:                 multiplier,
			ImageRateMultiplier:            imageMultiplier,
			ServiceTier:                    serviceTier,
			Resolver:                       s.resolver,
			Resolved:                       resolved,
			LongContextBillingEnabled:      &longContextBillingEnabled,
			DisableModelLongContextPricing: modelLongContextExempt,
		})
		return cost, resolved, err
	}
	cost, err := s.billingService.calculateCostInternalWithPolicy(
		billingModel,
		tokens,
		multiplier,
		imageMultiplier,
		serviceTier,
		nil,
		longContextBillingEnabled && !modelLongContextExempt,
	)
	return cost, nil, err
}

func (s *OpenAIGatewayService) calculateOpenAIImageCost(
	ctx context.Context,
	billingModel string,
	apiKey *APIKey,
	result *OpenAIForwardResult,
	multiplier float64,
) (*CostBreakdown, error) {
	// multimodal 是逐模型定价分组：图片必须命中分组或渠道显式配置的价格，
	// 不允许回落到 CalculateImageCost 内置的默认图片价。
	multimodal := IsMultimodal(PlatformFromAPIKey(apiKey))
	sizeTier := NormalizeImageBillingTierOrDefault(result.ImageSize)
	if multimodal {
		// 尺寸无法映射到计费档位时不能猜 2K：请求没给尺寸且上游也没回传
		// 可解析尺寸时直接报错，避免按错误档位计费。
		tier, ok := ClassifyImageBillingTier(result.ImageSize)
		if !ok {
			return nil, fmt.Errorf("image size cannot be mapped to a billing tier: model=%q size=%q", billingModel, result.ImageSize)
		}
		sizeTier = tier
	}
	resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey)
	// 逐模型定价分组必须精确命中当前尺寸档位的价格：
	// 只配了 1K 却请求 2K 时不允许借用 1K 档位或其它档位的价格。
	if multimodal && !imageUnitPriceConfigured(resolved, sizeTier) {
		return nil, fmt.Errorf("image price is not configured: model=%q size=%q", billingModel, sizeTier)
	}
	var cost *CostBreakdown
	if resolved != nil && resolved.Source == PricingSourceGroup &&
		(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
		gid := apiKey.Group.ID
		groupCost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: billingModel, GroupID: &gid, Group: apiKey.Group,
			Tokens:       UsageTokens{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens, ImageOutputTokens: result.Usage.ImageOutputTokens},
			RequestCount: result.ImageCount, SizeTier: sizeTier,
			RateMultiplier: multiplier, Resolver: s.resolver, Resolved: resolved,
		})
		if err == nil {
			cost = groupCost
		} else {
			logger.LegacyPrintf("service.openai_gateway", "Calculate image group cost failed: %v", err)
		}
	}
	groupConfig := imagePriceConfigFromAPIKey(apiKey)
	// multimodal 是逐模型定价分组：分组级 1K/2K/4K 图片价等同于兜底价，
	// 不参与计费，只能命中模型/渠道定价条目里的显式价格。
	if !multimodal {
		if cost == nil && apiKeyHasConfiguredImagePrice(apiKey, sizeTier) {
			cost = s.billingService.CalculateImageCost(billingModel, sizeTier, result.ImageCount, groupConfig, multiplier)
		}
		if cost == nil {
			if refreshed := s.apiKeyWithFreshGroupMediaPricing(ctx, apiKey); refreshed != apiKey {
				apiKey = refreshed
				groupConfig = imagePriceConfigFromAPIKey(apiKey)
				if apiKeyHasConfiguredImagePrice(apiKey, sizeTier) {
					cost = s.billingService.CalculateImageCost(billingModel, sizeTier, result.ImageCount, groupConfig, multiplier)
				}
			}
		}
	}
	if cost == nil && resolved != nil && resolved.Source == PricingSourceChannel &&
		(resolved.Mode == BillingModePerRequest || resolved.Mode == BillingModeImage) {
		gid := apiKey.Group.ID
		channelCost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Group:          apiKey.Group,
			Tokens:         UsageTokens{InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens, ImageOutputTokens: result.Usage.ImageOutputTokens},
			RequestCount:   result.ImageCount,
			SizeTier:       sizeTier,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
		if err == nil {
			cost = channelCost
		} else {
			logger.LegacyPrintf("service.openai_gateway", "Calculate image channel cost failed: %v", err)
		}
	}
	if cost == nil && !multimodal {
		cost = s.billingService.CalculateImageCost(billingModel, sizeTier, result.ImageCount, groupConfig, multiplier)
	}
	// 逐模型定价分组没有命中任何显式价格时直接报错，不做价格猜测。
	if cost == nil || (multimodal && cost.TotalCost <= 0) {
		return nil, fmt.Errorf("image price is not configured: model=%q size=%q", billingModel, sizeTier)
	}
	return cost, nil
}

// imageUnitPriceConfigured 报告图片定价是否为当前尺寸档位配置了单价。
// 只认该档位的显式层级价：multimodal 没有“默认价”，
// 图片档位也只能按尺寸命中，不能回退到默认价或其它档位。
func imageUnitPriceConfigured(resolved *ResolvedPricing, sizeTier string) bool {
	if resolved == nil {
		return false
	}
	for _, tier := range resolved.RequestTiers {
		if tier.TierLabel == sizeTier && tier.PerRequestPrice != nil {
			return true
		}
	}
	return false
}

// MultimodalImagePricingRequest 描述一次图片生成的前置价格校验输入。
type MultimodalImagePricingRequest struct {
	APIKey  *APIKey
	UserID  int64
	GroupID int64
	Model   string
	Size    string
	Count   int
}

// ValidateMultimodalImagePricing 在提交上游前校验图片单价可解析。
// 图片用量由异步任务记录，RecordUsage 的缺价错误只会写日志、无法让请求失败，
// 因此必须在转发前拦截，避免用户拿到未计费的图片。
func (s *OpenAIGatewayService) ValidateMultimodalImagePricing(ctx context.Context, req MultimodalImagePricingRequest) error {
	if s == nil || s.billingService == nil || req.APIKey == nil {
		return nil
	}
	// 简单模式只记录用量、不扣费，不因缺少价格拒绝生成。
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	model := strings.TrimSpace(req.Model)
	// token 计费的图片模型由 token 用量结算，提交前给不出按次价，交由用量记录处理。
	// multimodal 是逐模型定价分组：只有显式配成 token 计费的条目才允许放行，
	// 否则解析不到条目时会退化成 LiteLLM token 价，等于给未配置价格的模型免费出图。
	if resolved := s.resolveOpenAIChannelPricing(ctx, model, req.APIKey); resolved != nil &&
		resolved.Mode == BillingModeToken &&
		(!IsMultimodal(PlatformFromAPIKey(req.APIKey)) ||
			resolved.Source == PricingSourceGroup ||
			resolved.Source == PricingSourceChannel) {
		return nil
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	// 请求没有给出可分类尺寸（未传 size / auto / 非标准尺寸）时不能猜 2K：
	// 实际计费档位只能由上游输出尺寸确定，任意档位都可能成为本次请求的结算档位，
	// 因此必须所有档位都配价才允许提交。只配了部分档位时直接拒绝，
	// 而不是先出图、再在落账阶段因为实际档位缺价而漏账。
	if _, ok := ClassifyImageBillingTier(req.Size); !ok {
		if !s.multimodalImageAllTierPricesConfigured(ctx, model, req.APIKey) {
			return ErrImagePriceNotConfigured
		}
		return nil
	}
	multiplier := s.multimodalImageRateMultiplier(ctx, req.APIKey, req.UserID, req.GroupID)
	cost, err := s.calculateOpenAIImageCost(ctx, model, req.APIKey, &OpenAIForwardResult{
		ImageCount: count,
		ImageSize:  req.Size,
	}, multiplier)
	if err != nil {
		return ErrImagePriceNotConfigured.WithCause(err)
	}
	if cost == nil {
		return ErrImagePriceNotConfigured
	}
	return nil
}

// multimodalImageAllTierPricesConfigured 报告模型是否为所有图片档位都配置了单价。
// 请求未指定可分类尺寸时，上游可能返回任意档位的图片；只要有一个档位缺价，
// 该请求就可能以无法结算的档位交付，因此必须全部有价才允许提交。
func (s *OpenAIGatewayService) multimodalImageAllTierPricesConfigured(ctx context.Context, model string, apiKey *APIKey) bool {
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	if resolved == nil {
		return false
	}
	for _, tier := range []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K} {
		if !imageUnitPriceConfigured(resolved, tier) {
			return false
		}
	}
	return true
}

// validateMultimodalImageSettlement 在把上游图片结果写回客户端之前，
// 用与 RecordUsage 一致的档位解析确认实际计费档位已配置单价。
// 请求未指定可分类尺寸时，档位只能在拿到上游响应后确定；
// 若该档位缺价，必须在这里失败，不能先交付图片再让落账阶段的缺价错误只写日志。
func (s *OpenAIGatewayService) validateMultimodalImageSettlement(
	ctx context.Context,
	model string,
	apiKey *APIKey,
	usage multimodalMediaUsageMetadata,
) error {
	if s == nil || s.billingService == nil || apiKey == nil || usage.ImageCount <= 0 {
		return nil
	}
	// 简单模式只记录用量、不扣费，不因缺少价格拒绝交付。
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return nil
	}
	model = strings.TrimSpace(model)
	// token 计费的图片模型由 token 用量结算，不存在按次图片价。
	if resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey); resolved != nil &&
		resolved.Mode == BillingModeToken &&
		(!IsMultimodal(PlatformFromAPIKey(apiKey)) ||
			resolved.Source == PricingSourceGroup ||
			resolved.Source == PricingSourceChannel) {
		return nil
	}
	probe := &OpenAIForwardResult{
		ImageCount:             usage.ImageCount,
		ImageSize:              usage.ImageSize,
		ImageInputSize:         usage.ImageInputSize,
		ImageOutputSizes:       usage.ImageOutputSizes,
		ImageBillingSizeLocked: usage.ImageBillingSizeLocked,
	}
	if !probe.ImageBillingSizeLocked {
		ApplyOpenAIImageBillingResolution(probe)
	}
	tier, ok := ClassifyImageBillingTier(probe.ImageSize)
	if !ok {
		return ErrImagePriceNotConfigured.WithCause(fmt.Errorf(
			"image size cannot be mapped to a billing tier: model=%q size=%q", model, probe.ImageSize))
	}
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	if !imageUnitPriceConfigured(resolved, tier) {
		return ErrImagePriceNotConfigured.WithCause(fmt.Errorf(
			"image price is not configured: model=%q size=%q", model, tier))
	}
	return nil
}

// multimodalImageRateMultiplier 复刻 RecordUsage 的图片倍率口径：
// 分组默认倍率或用户专属倍率，图片倍率独立时改用图片倍率。
func (s *OpenAIGatewayService) multimodalImageRateMultiplier(
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
	return resolveImageRateMultiplier(apiKey, baseMultiplier)
}

func (s *OpenAIGatewayService) calculateOpenAIVideoCost(
	ctx context.Context,
	billingModel string,
	apiKey *APIKey,
	result *OpenAIForwardResult,
	multiplier float64,
) (*CostBreakdown, error) {
	videoCount := result.VideoCount
	if videoCount <= 0 {
		videoCount = 1
	}
	resolution := NormalizeVideoBillingResolutionOrDefault(result.VideoResolution)
	durationSeconds := NormalizeVideoBillingDurationSecondsOrDefault(result.VideoDurationSeconds)
	resolved := s.resolveOpenAIChannelPricing(ctx, billingModel, apiKey)
	// multimodal 是逐模型定价分组：视频每秒价必须写在模型/渠道定价条目里，
	// 分组级视频价不参与计费。条目缺价时不能静默按 0 元放行，最终仍无价则返回 503。
	multimodal := IsMultimodal(PlatformFromAPIKey(apiKey))
	videoPriceConfigured := func(resolved *ResolvedPricing) bool {
		return !multimodal || videoUnitPriceConfigured(resolved, resolution)
	}
	if resolved != nil && resolved.Source == PricingSourceGroup && resolved.Mode == BillingModeVideo && videoPriceConfigured(resolved) {
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx: ctx, Model: billingModel, GroupID: &gid, Group: apiKey.Group,
			UsageUnits: float64(videoCount * durationSeconds), SizeTier: resolution,
			RateMultiplier: multiplier, Resolver: s.resolver, Resolved: resolved,
		})
		if err == nil {
			return cost, nil
		}
		logger.LegacyPrintf("service.openai_gateway", "Calculate video group cost failed: %v", err)
	}
	// groupConfig 保留 Platform，让 multimodal 在 CalculateVideoCost 中明确无价，
	// 而不是因为缺少平台信息误落到 Grok 内置每秒价。
	groupConfig := videoPriceConfigFromAPIKey(apiKey)
	if !multimodal {
		if apiKeyHasConfiguredVideoPrice(apiKey, resolution) {
			return s.billingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, groupConfig, multiplier)
		}
		if refreshed := s.apiKeyWithFreshGroupMediaPricing(ctx, apiKey); refreshed != apiKey {
			apiKey = refreshed
			groupConfig = videoPriceConfigFromAPIKey(apiKey)
			if apiKeyHasConfiguredVideoPrice(apiKey, resolution) {
				return s.billingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, groupConfig, multiplier)
			}
		}
	}
	// 视频只允许按秒计费：渠道的 per_request / image 定价是按次口径，不能套到视频请求上，
	// 否则 4 秒和 8 秒会收同样的钱。命中不到按秒价时继续向下取分组视频价，
	// 最终仍无价则由 CalculateVideoCost 返回错误，绝不退化成按次收费。
	if resolved != nil && resolved.Source == PricingSourceChannel && resolved.Mode == BillingModeVideo && videoPriceConfigured(resolved) {
		gid := apiKey.Group.ID
		cost, err := s.billingService.CalculateCostUnified(CostInput{
			Ctx:            ctx,
			Model:          billingModel,
			GroupID:        &gid,
			Group:          apiKey.Group,
			RequestCount:   videoCount,
			UsageUnits:     float64(videoCount * durationSeconds),
			SizeTier:       resolution,
			RateMultiplier: multiplier,
			Resolver:       s.resolver,
			Resolved:       resolved,
		})
		if err == nil {
			cost.BillingMode = string(BillingModeVideo)
			return cost, nil
		}
		logger.LegacyPrintf("service.openai_gateway", "Calculate video channel cost failed: %v", err)
	}

	return s.billingService.CalculateVideoCost(billingModel, resolution, videoCount, durationSeconds, groupConfig, multiplier)
}

// videoUnitPriceConfigured 报告视频定价是否真的为当前分辨率配置了每秒价。
// 视频层级只能按分辨率命中，不像 token 区间那样有上下文回退，因此这里必须显式判断，
// 避免 multimodal 分组的空视频条目被当成 0 元价格放行。
// multimodal 没有“默认价”：只认该分辨率的显式层级价，条目级默认每秒价同样不认。
func videoUnitPriceConfigured(resolved *ResolvedPricing, resolution string) bool {
	if resolved == nil {
		return false
	}
	for _, tier := range resolved.RequestTiers {
		if tier.TierLabel == resolution && tier.PerRequestPrice != nil {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) apiKeyWithFreshGroupMediaPricing(ctx context.Context, apiKey *APIKey) *APIKey {
	if apiKey == nil || apiKey.GroupID == nil || *apiKey.GroupID <= 0 {
		return apiKey
	}
	if !groupMediaPricingLooksIncomplete(apiKey.Group) {
		return apiKey
	}
	if s == nil || s.channelService == nil || s.channelService.groupRepo == nil {
		return apiKey
	}
	group, err := s.channelService.groupRepo.GetByIDLite(ctx, *apiKey.GroupID)
	if err != nil || group == nil {
		return apiKey
	}
	clone := *apiKey
	clone.Group = group
	return &clone
}

// groupMediaPricingLooksIncomplete 判断分组对象是否可能缺失媒体计费字段（例如由不含
// 这些字段的旧快照或手工构造的上下文对象生成）。image/video 独立倍率在数据库中的
// 默认值均为 1.0，正常加载的分组不可能两个倍率同时为 0 且未开启独立倍率、全部媒体
// 价为 nil——只有这种情况才回源查库，避免对未配置覆盖价的分组每条媒体用量都多打一次 DB 查询。
func groupMediaPricingLooksIncomplete(group *Group) bool {
	if group == nil {
		return true
	}
	if group.ImageRateIndependent || group.VideoRateIndependent {
		return false
	}
	if group.ImageRateMultiplier != 0 || group.VideoRateMultiplier != 0 {
		return false
	}
	if len(group.ModelPricing) > 0 || group.LongContextPricingEnabled {
		return false
	}
	return group.ImagePrice1K == nil && group.ImagePrice2K == nil && group.ImagePrice4K == nil &&
		group.VideoPrice480P == nil && group.VideoPrice720P == nil && group.VideoPrice1080P == nil
}

func (s *OpenAIGatewayService) resolveOpenAIBillingPricing(ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	if s.resolver == nil || apiKey == nil || apiKey.Group == nil {
		return nil
	}
	gid := apiKey.Group.ID
	return s.resolver.Resolve(ctx, PricingInput{Model: billingModel, GroupID: &gid, Group: apiKey.Group})
}

func (s *OpenAIGatewayService) resolveOpenAIChannelPricing(ctx context.Context, billingModel string, apiKey *APIKey) *ResolvedPricing {
	resolved := s.resolveOpenAIBillingPricing(ctx, billingModel, apiKey)
	if resolved == nil {
		return nil
	}
	if isExplicitAdminPricing(resolved) {
		return resolved
	}
	return nil
}

// ParseCodexRateLimitHeaders extracts Codex usage limits from response headers.
// Exported for use in ratelimit_service when handling OpenAI 429 responses.
func ParseCodexRateLimitHeaders(headers http.Header) *OpenAICodexUsageSnapshot {
	snapshot := &OpenAICodexUsageSnapshot{}
	hasData := false

	// Helper to parse float64 from header
	parseFloat := func(key string) *float64 {
		if v := headers.Get(key); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return &f
			}
		}
		return nil
	}

	// Helper to parse int from header
	parseInt := func(key string) *int {
		if v := headers.Get(key); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				return &i
			}
		}
		return nil
	}

	// Primary (weekly) limits
	if v := parseFloat("x-codex-primary-used-percent"); v != nil {
		snapshot.PrimaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-reset-after-seconds"); v != nil {
		snapshot.PrimaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-primary-window-minutes"); v != nil {
		snapshot.PrimaryWindowMinutes = v
		hasData = true
	}

	// Secondary (5h) limits
	if v := parseFloat("x-codex-secondary-used-percent"); v != nil {
		snapshot.SecondaryUsedPercent = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-reset-after-seconds"); v != nil {
		snapshot.SecondaryResetAfterSeconds = v
		hasData = true
	}
	if v := parseInt("x-codex-secondary-window-minutes"); v != nil {
		snapshot.SecondaryWindowMinutes = v
		hasData = true
	}

	// Overflow ratio
	if v := parseFloat("x-codex-primary-over-secondary-limit-percent"); v != nil {
		snapshot.PrimaryOverSecondaryPercent = v
		hasData = true
	}

	if !hasData {
		return nil
	}

	snapshot.UpdatedAt = time.Now().Format(time.RFC3339)
	return snapshot
}

func codexSnapshotBaseTime(snapshot *OpenAICodexUsageSnapshot, fallback time.Time) time.Time {
	if snapshot == nil {
		return fallback
	}
	if snapshot.UpdatedAt == "" {
		return fallback
	}
	base, err := time.Parse(time.RFC3339, snapshot.UpdatedAt)
	if err != nil {
		return fallback
	}
	return base
}

func codexResetAtRFC3339(base time.Time, resetAfterSeconds *int) *string {
	if resetAfterSeconds == nil {
		return nil
	}
	sec := *resetAfterSeconds
	if sec < 0 {
		sec = 0
	}
	resetAt := base.Add(time.Duration(sec) * time.Second).Format(time.RFC3339)
	return &resetAt
}

func buildCodexUsageExtraUpdates(snapshot *OpenAICodexUsageSnapshot, fallbackNow time.Time) map[string]any {
	if snapshot == nil {
		return nil
	}

	baseTime := codexSnapshotBaseTime(snapshot, fallbackNow)
	updates := make(map[string]any)

	// 保存原始 primary/secondary 字段，便于排查问题
	if snapshot.PrimaryUsedPercent != nil {
		updates["codex_primary_used_percent"] = *snapshot.PrimaryUsedPercent
	}
	if snapshot.PrimaryResetAfterSeconds != nil {
		updates["codex_primary_reset_after_seconds"] = *snapshot.PrimaryResetAfterSeconds
	}
	if snapshot.PrimaryWindowMinutes != nil {
		updates["codex_primary_window_minutes"] = *snapshot.PrimaryWindowMinutes
	}
	if snapshot.SecondaryUsedPercent != nil {
		updates["codex_secondary_used_percent"] = *snapshot.SecondaryUsedPercent
	}
	if snapshot.SecondaryResetAfterSeconds != nil {
		updates["codex_secondary_reset_after_seconds"] = *snapshot.SecondaryResetAfterSeconds
	}
	if snapshot.SecondaryWindowMinutes != nil {
		updates["codex_secondary_window_minutes"] = *snapshot.SecondaryWindowMinutes
	}
	if snapshot.PrimaryOverSecondaryPercent != nil {
		updates["codex_primary_over_secondary_percent"] = *snapshot.PrimaryOverSecondaryPercent
	}
	updates["codex_usage_updated_at"] = baseTime.Format(time.RFC3339)

	// 归一化到 5h/7d 规范字段
	if normalized := snapshot.Normalize(); normalized != nil {
		if normalized.Used5hPercent != nil {
			updates["codex_5h_used_percent"] = *normalized.Used5hPercent
		}
		if normalized.Reset5hSeconds != nil {
			updates["codex_5h_reset_after_seconds"] = *normalized.Reset5hSeconds
		}
		if normalized.Window5hMinutes != nil {
			updates["codex_5h_window_minutes"] = *normalized.Window5hMinutes
		}
		if normalized.Used7dPercent != nil {
			updates["codex_7d_used_percent"] = *normalized.Used7dPercent
		}
		if normalized.Reset7dSeconds != nil {
			updates["codex_7d_reset_after_seconds"] = *normalized.Reset7dSeconds
		}
		if normalized.Window7dMinutes != nil {
			updates["codex_7d_window_minutes"] = *normalized.Window7dMinutes
		}
		if reset5hAt := codexResetAtRFC3339(baseTime, normalized.Reset5hSeconds); reset5hAt != nil {
			updates["codex_5h_reset_at"] = *reset5hAt
		}
		if reset7dAt := codexResetAtRFC3339(baseTime, normalized.Reset7dSeconds); reset7dAt != nil {
			updates["codex_7d_reset_at"] = *reset7dAt
		}
	}

	return updates
}

// updateCodexUsageSnapshot saves the Codex usage snapshot to account's Extra field
// updateCodexUsageSnapshot 把 /responses 的 x-codex-* 全局头快照写入账号 codex_* Extra。
// ⚠️ 调用方必须排除 spark 影子账号(account.IsShadow()):影子的 codex_* 仅由 QueryUsage
// (/wham/usage bengalfox 道)更新,不能被全局头口径污染(外审第7轮 P1)。本函数仅持 accountID,
// 无法在此自检影子,故守卫前置到各调用点。
func (s *OpenAIGatewayService) updateCodexUsageSnapshot(ctx context.Context, accountID int64, snapshot *OpenAICodexUsageSnapshot) {
	if snapshot == nil {
		return
	}
	if s == nil || s.accountRepo == nil {
		return
	}

	now := time.Now()
	updates := buildCodexUsageExtraUpdates(snapshot, now)
	if len(updates) == 0 {
		return
	}
	if !s.getCodexSnapshotThrottle().Allow(accountID, now) {
		return
	}
	maybeRecoverOpenAICodexQuotaModelRateLimits(ctx, s.accountRepo, accountID, updates)

	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.accountRepo.UpdateExtra(updateCtx, accountID, updates)
	}()
}

func (s *OpenAIGatewayService) UpdateCodexUsageSnapshotFromHeaders(ctx context.Context, accountID int64, headers http.Header) {
	if accountID <= 0 || headers == nil {
		return
	}
	if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
		s.updateCodexUsageSnapshot(ctx, accountID, snapshot)
	}
}
