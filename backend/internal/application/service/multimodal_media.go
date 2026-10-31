package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Multimodal 平台是独立的媒体直通链路：同一 API Key 覆盖图片与视频全部端点，
// 上游以 OpenAI 兼容的媒体协议暴露 /v1/images/* 与 /v1/videos/*。
// 与 Grok 媒体链路的差异：
//   - base_url 完全由账号配置决定，不走 xAI 专用校验器；
//   - 生图必须强制 stream=true 绕过 Cloudflare 100 秒硬超时，因此需要把 SSE
//     聚合回普通 JSON（客户端显式要求流式时原样透传）；
//   - 取片端点允许上游 302 跳转 CDN，Location 直接回写给客户端；
//   - 错误处理复用 OpenAI 账号运行时，不做 Grok 内容策略特判。

type MultimodalMediaEndpoint string

const (
	MultimodalMediaEndpointImagesGenerations MultimodalMediaEndpoint = "images_generations"
	MultimodalMediaEndpointImagesEdits       MultimodalMediaEndpoint = "images_edits"
	MultimodalMediaEndpointVideosGenerations MultimodalMediaEndpoint = "videos_generations"
	MultimodalMediaEndpointVideosEdits       MultimodalMediaEndpoint = "videos_edits"
	MultimodalMediaEndpointVideosExtensions  MultimodalMediaEndpoint = "videos_extensions"
	MultimodalMediaEndpointVideoStatus       MultimodalMediaEndpoint = "video_status"
	MultimodalMediaEndpointVideoContent      MultimodalMediaEndpoint = "video_content"
)

const (
	multimodalVideosGenerationsPath = "/v1/videos/generations"
	multimodalVideosEditsPath       = "/v1/videos/edits"
	multimodalVideosExtensionsPath  = "/v1/videos/extensions"

	multimodalVideoRequestSessionPrefix = "multimodal-video:"
)

func (e MultimodalMediaEndpoint) RequiresRequestBody() bool {
	return !e.IsVideoLookupRequest()
}

func (e MultimodalMediaEndpoint) IsVideoLookupRequest() bool {
	return e == MultimodalMediaEndpointVideoStatus || e == MultimodalMediaEndpointVideoContent
}

func (e MultimodalMediaEndpoint) IsGenerationRequest() bool {
	switch e {
	case MultimodalMediaEndpointImagesGenerations, MultimodalMediaEndpointImagesEdits,
		MultimodalMediaEndpointVideosGenerations, MultimodalMediaEndpointVideosEdits,
		MultimodalMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}

func (e MultimodalMediaEndpoint) IsImageRequest() bool {
	return e == MultimodalMediaEndpointImagesGenerations || e == MultimodalMediaEndpointImagesEdits
}

func (e MultimodalMediaEndpoint) IsVideoGenerationRequest() bool {
	switch e {
	case MultimodalMediaEndpointVideosGenerations, MultimodalMediaEndpointVideosEdits,
		MultimodalMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}

func (e MultimodalMediaEndpoint) httpMethod() string {
	if e.IsVideoLookupRequest() {
		return http.MethodGet
	}
	return http.MethodPost
}

// multimodalMediaClientEndpointPath 返回媒体端点对应的客户端 API 路径，
// 用于自定义模型 request_adapter 的 match.endpoint 判定与 {{request.endpoint}} 变量。
func multimodalMediaClientEndpointPath(endpoint MultimodalMediaEndpoint) string {
	switch endpoint {
	case MultimodalMediaEndpointImagesGenerations:
		return openAIImagesGenerationsEndpoint
	case MultimodalMediaEndpointImagesEdits:
		return openAIImagesEditsEndpoint
	case MultimodalMediaEndpointVideosGenerations:
		return multimodalVideosGenerationsPath
	case MultimodalMediaEndpointVideosEdits:
		return multimodalVideosEditsPath
	case MultimodalMediaEndpointVideosExtensions:
		return multimodalVideosExtensionsPath
	default:
		return ""
	}
}

// DefaultMultimodalModelIDs 是上游媒体分组当前公开的模型目录，
// 供 /v1/models 在尚未同步上游列表时回退，以及分组/账号白名单预填。
func DefaultMultimodalModelIDs() []string {
	return []string{
		// 图片
		"gpt-image-2.5",
		"gpt-image-2.5-flare",
		"gpt-image-2.5-sunburst",
		"gpt-image-2",
		"seedream",
		"seedream5",
		"grok-imagine-image",
		"grok-imagine-image-2.0",
		"grok-imagine-image-lite",
		"grok-imagine-image-quality",
		"grok-imagine-image-edit",
		// 视频
		"seedance",
		"seedance2.0",
		"seedance2.0-fast",
		"seedance2.0-mini",
		"seedance2.5",
		"dreamina-seedance-2-0",
		"dreamina-seedance-2-5",
		"dreamina-seedance-2-0-mini-260615",
		"dreamina-seedance-2-0-fast-260128",
		"dreamina-seedance-2-0-260128",
		"dreamina-seedance-2-5-260628",
		"dreamina-seedance-2-0-hc",
		"dreamina-seedance-2-5-hc",
		"grok-imagine-video",
		"grok-imagine-video-1.5",
	}
}

func multimodalMediaUpstreamPath(endpoint MultimodalMediaEndpoint, requestID string) (string, error) {
	switch endpoint {
	case MultimodalMediaEndpointImagesGenerations:
		return openAIImagesGenerationsEndpoint, nil
	case MultimodalMediaEndpointImagesEdits:
		return openAIImagesEditsEndpoint, nil
	// 上游媒体分组只公开 /v1/videos/generations；视频编辑与续写都通过
	// referenceVideos 在该端点上表达，上游没有独立的 edits/extensions 路径。
	case MultimodalMediaEndpointVideosGenerations,
		MultimodalMediaEndpointVideosEdits,
		MultimodalMediaEndpointVideosExtensions:
		return multimodalVideosGenerationsPath, nil
	case MultimodalMediaEndpointVideoStatus:
		return "/v1/videos/" + url.PathEscape(strings.Trim(requestID, "/")), nil
	case MultimodalMediaEndpointVideoContent:
		return "/v1/videos/" + url.PathEscape(strings.Trim(requestID, "/")) + "/content", nil
	default:
		return "", fmt.Errorf("unsupported multimodal media endpoint: %s", endpoint)
	}
}

func buildMultimodalMediaURL(account *Account, endpoint MultimodalMediaEndpoint, requestID string) (string, error) {
	if account == nil || !account.IsMultimodal() {
		return "", fmt.Errorf("multimodal account is required")
	}
	if endpoint.IsVideoLookupRequest() && strings.TrimSpace(requestID) == "" {
		return "", fmt.Errorf("multimodal media request id is required")
	}
	path, err := multimodalMediaUpstreamPath(endpoint, requestID)
	if err != nil {
		return "", err
	}
	baseURL := strings.TrimSpace(account.GetMultimodalBaseURL())
	if baseURL == "" {
		return "", fmt.Errorf("multimodal account base_url is required")
	}
	return buildOpenAIEndpointURL(baseURL, path), nil
}

// MultimodalMediaVideoRequestSessionHash 为视频任务生成稳定的归属会话哈希。
// 使用独立前缀，避免与 Grok 视频任务在同一 sticky session 缓存中串号。
func MultimodalMediaVideoRequestSessionHash(requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	ownerSeed := fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID)
	return multimodalVideoRequestSessionPrefix + DeriveSessionHashFromSeed(ownerSeed)
}

// BindMultimodalMediaVideoRequestAccount 记录视频任务创建时使用的账号，
// 后续轮询与取片必须回到同一账号，否则上游任务不可见。
func (s *OpenAIGatewayService) BindMultimodalMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID, accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("multimodal video request binding cache is unavailable")
	}
	sessionHash := MultimodalMediaVideoRequestSessionHash(requestID, userID, apiKeyID)
	cacheKey := s.openAISessionCacheKey(sessionHash)
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("multimodal video request binding is invalid")
	}
	ttl := openaiStickySessionTTL
	if s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		ttl = time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, accountID, ttl)
}

func (s *OpenAIGatewayService) ResolveMultimodalMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID int64,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("multimodal video request binding cache is unavailable")
	}
	sessionHash := MultimodalMediaVideoRequestSessionHash(requestID, userID, apiKeyID)
	cacheKey := s.openAISessionCacheKey(sessionHash)
	if cacheKey == "" {
		return 0, fmt.Errorf("multimodal video request binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

// SelectMultimodalMediaVideoRequestAccount 只接受任务创建时绑定的账号。
// 通用 sticky 选号可能逃逸到其它可用账号并覆盖归属语义，因此这里显式禁用逃逸。
func (s *OpenAIGatewayService) SelectMultimodalMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	accountID int64,
	requestedModel string,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerSessionSticky}
	if s == nil || accountID <= 0 || strings.TrimSpace(sessionHash) == "" {
		return nil, decision, ErrNoAvailableAccounts
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = WithOpenAIProfitControlSuppressed(ctx)
	scheduler := &defaultOpenAIAccountScheduler{service: s}
	selection, _, err := scheduler.selectBySessionHash(ctx, OpenAIAccountScheduleRequest{
		GroupID:               groupID,
		Platform:              PlatformMultimodal,
		SessionHash:           sessionHash,
		StickyAccountID:       accountID,
		PreserveStickyBinding: true,
		DisableStickyEscape:   true,
		RequestedModel:        requestedModel,
		RequiredTransport:     OpenAIUpstreamTransportHTTPSSE,
	})
	if err != nil {
		return nil, decision, err
	}
	if selection == nil || selection.Account == nil {
		return nil, decision, ErrNoAvailableAccounts
	}
	decision.StickySessionHit = true
	decision.SelectedAccountID = selection.Account.ID
	decision.SelectedAccountType = selection.Account.Type
	return selection, decision, nil
}

func (s *OpenAIGatewayService) ForwardMultimodalMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint MultimodalMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
	videoHold *BalanceHoldRecord,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("multimodal account is required")
	}
	if account.Platform != PlatformMultimodal {
		return nil, fmt.Errorf("account platform %s is not supported for multimodal media", account.Platform)
	}
	targetURL, err := buildMultimodalMediaURL(account, endpoint, requestID)
	if err != nil {
		return nil, err
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	if endpoint == MultimodalMediaEndpointVideoContent {
		return s.forwardMultimodalMediaVideoContent(ctx, c, account, token, requestID, startTime)
	}

	// Preserve client parameters before the upstream adapter forces SSE or maps
	// fields. Response format and billing must use the original request intent.
	requestInfo := ParseGrokMediaRequest(contentType, body)
	clientWantsStream := endpoint.IsImageRequest() && multimodalMediaClientWantsStream(body, contentType)
	if endpoint.IsVideoGenerationRequest() {
		if err := ValidateMultimodalVideoResolution(requestInfo.ResolutionRaw); err != nil {
			return nil, err
		}
	}
	body, contentType, err = forceMultimodalImageUpstreamStream(endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	body, err = normalizeMultimodalVideoReferenceBody(endpoint, body)
	if err != nil {
		return nil, err
	}
	upstreamModel := requestInfo.Model
	if endpoint.RequiresRequestBody() && gjson.ValidBytes(body) {
		if mappedModel := strings.TrimSpace(account.GetMappedModel(requestInfo.Model)); mappedModel != "" {
			upstreamModel = mappedModel
		}
		if upstreamModel != requestInfo.Model {
			body, err = sjson.SetBytes(body, "model", upstreamModel)
			if err != nil {
				return nil, fmt.Errorf("rewrite multimodal media account mapped model: %w", err)
			}
		}
	}
	if endpoint.RequiresRequestBody() && s.customModelCapabilities != nil {
		modality := "video"
		if endpoint.IsImageRequest() {
			modality = "image"
		}
		body, err = applyModelAttributes(
			ctx,
			s.customModelCapabilities,
			requestInfo.Model,
			modality,
			nil,
			body,
			contentType,
		)
		if err != nil {
			return nil, err
		}
	}
	var adapterHeaders map[string]string
	if endpoint.RequiresRequestBody() && s.customModelCapabilities != nil {
		requestAdapter, configured, resolveErr := s.resolveCustomModelRequestAdapter(
			ctx,
			requestInfo.Model,
			upstreamModel,
		)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if configured {
			clientEndpoint := multimodalMediaClientEndpointPath(endpoint)
			adapted, adaptErr := applyCustomModelRequestAdapter(
				body,
				contentType,
				clientEndpoint,
				buildGrokMediaRequestVariables(requestInfo, clientEndpoint, body, contentType, upstreamModel),
				requestAdapter,
				func() (map[string]any, error) { return customModelJSONAdapterBase(body, contentType) },
			)
			if adaptErr != nil {
				return nil, adaptErr
			}
			body = adapted.Body
			contentType = adapted.ContentType
			adapterHeaders = adapted.Headers
			if adapted.Endpoint != clientEndpoint {
				targetURL, err = replaceGrokMediaURLPath(targetURL, adapted.Endpoint)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if endpoint.RequiresRequestBody() {
		contentType = strings.TrimSpace(contentType)
		if contentType == "" {
			contentType = "application/json"
		}
		upstreamReq.Header.Set("Content-Type", contentType)
	}
	// 自定义模型模板请求头先应用，账号级请求头覆写最后应用并优先。
	for name, value := range adapterHeaders {
		upstreamReq.Header.Set(resolveWireCasing(name), value)
	}
	account.ApplyHeaderOverrides(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := doAccountHTTPUpstream(s.httpUpstream, upstreamReq, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	if resp.StatusCode >= 400 {
		return s.handleMultimodalMediaErrorResponse(ctx, resp, c, account, requestIDHeader, upstreamModel)
	}

	responseContentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	var respBody []byte
	var imageSSEUsage *multimodalMediaUsageMetadata
	var firstTokenMs *int
	if endpoint.IsImageRequest() && multimodalMediaIsEventStream(responseContentType) {
		var usage multimodalMediaUsageMetadata
		respBody, usage, firstTokenMs, err = s.readMultimodalImageSSE(ctx, c, resp, requestInfo, clientWantsStream, startTime)
		if err != nil {
			if clientWantsStream && IsResponseCommitted(c) {
				// Already delivered images still require settlement if a later read
				// fails. The handler must not fail over a committed stream.
				return multimodalMediaForwardResult(requestIDHeader, upstreamModel, requestInfo, usage, resp.Header, startTime, true, firstTokenMs), err
			}
			return nil, err
		}
		if clientWantsStream {
			return multimodalMediaForwardResult(requestIDHeader, upstreamModel, requestInfo, usage, resp.Header, startTime, true, firstTokenMs), nil
		} else {
			imageSSEUsage = &usage
			responseContentType = "application/json"
		}
	} else {
		respBody, err = ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
		if err != nil {
			return nil, err
		}
	}
	if endpoint.IsVideoGenerationRequest() || endpoint == MultimodalMediaEndpointVideoStatus {
		respBody = renderMultimodalVideoResponse(endpoint, requestInfo, body, respBody)
	}
	if endpoint == MultimodalMediaEndpointVideoStatus {
		respBody = rewriteMultimodalVideoContentURLs(respBody, requestID, multimodalMediaContentProxyURL(c, requestID))
	}
	// 视频两段式计费必须在写回响应前完成：创建阶段绑定冻结记录并回显冻结金额，
	// 轮询阶段结算或释放，并把最终扣费金额一起返回给客户端。
	if endpoint.IsVideoGenerationRequest() || endpoint == MultimodalMediaEndpointVideoStatus {
		rendered, billingErr := s.applyMultimodalVideoBilling(ctx, c, account, endpoint, requestID, videoHold, respBody)
		if billingErr != nil {
			// 绑定或结算失败：不写回响应，交给 handler 返回错误并释放冻结。
			return nil, billingErr
		}
		respBody = rendered
	}
	if endpoint.IsVideoGenerationRequest() && videoHold == nil {
		if err := s.bindCompositeMultimodalVideoOwner(ctx, c, account, respBody); err != nil {
			return nil, err
		}
	}
	if endpoint.IsImageRequest() {
		if countOpenAIResponseImageOutputsFromJSONBytes(respBody) <= 0 {
			setOpsUpstreamError(c, http.StatusBadGateway, "multimodal upstream returned no image output", truncateString(string(respBody), 512))
			return nil, &UpstreamFailoverError{
				StatusCode:      http.StatusBadGateway,
				ResponseBody:    respBody,
				ResponseHeaders: resp.Header.Clone(),
			}
		}
	}
	usage := multimodalMediaUsageFromResponse(endpoint, requestInfo, body, respBody)
	if imageSSEUsage != nil {
		usage = *imageSSEUsage
	}
	// 图片最终计费档位由上游实际输出决定：必须在写回响应前确认该档位已配置价格，
	// 否则客户端会先拿到无法结算的图片，缺价错误只会在落账阶段写日志。
	if endpoint.IsImageRequest() {
		if priceErr := s.validateMultimodalImageSettlement(ctx, requestInfo.Model, getAPIKeyFromContext(c), usage); priceErr != nil {
			return nil, priceErr
		}
	}
	writeMultimodalMediaResponse(c, resp.StatusCode, responseContentType, respBody, resp.Header, s.responseHeaderFilter)
	return multimodalMediaForwardResult(requestIDHeader, upstreamModel, requestInfo, usage, resp.Header, startTime, clientWantsStream, firstTokenMs), nil
}

func multimodalMediaForwardResult(requestID, upstreamModel string, requestInfo GrokMediaRequestInfo, usage multimodalMediaUsageMetadata, header http.Header, startTime time.Time, stream bool, firstTokenMs *int) *OpenAIForwardResult {
	return &OpenAIForwardResult{
		RequestID:              requestID,
		ResponseID:             usage.ResponseID,
		Usage:                  usage.Usage,
		Model:                  requestInfo.Model,
		BillingModel:           requestInfo.Model,
		UpstreamModel:          upstreamModel,
		ResponseHeaders:        header.Clone(),
		Duration:               time.Since(startTime),
		Stream:                 stream,
		FirstTokenMs:           firstTokenMs,
		ImageCount:             usage.ImageCount,
		ImageSize:              usage.ImageSize,
		ImageInputSize:         usage.ImageInputSize,
		ImageOutputSizes:       usage.ImageOutputSizes,
		ImageBillingSizeLocked: usage.ImageBillingSizeLocked,
		VideoCount:             usage.VideoCount,
		VideoResolution:        usage.VideoResolution,
		VideoDurationSeconds:   usage.VideoDurationSeconds,
	}
}

func (s *OpenAIGatewayService) forwardMultimodalMediaVideoContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token, requestID string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	contentURL, err := buildMultimodalMediaURL(account, MultimodalMediaEndpointVideoContent, requestID)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	contentReq, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(upstreamCtx),
		http.MethodGet,
		contentURL,
		nil,
	)
	if err != nil {
		return nil, err
	}
	contentReq.Header.Set("Accept", "*/*")
	if c != nil {
		if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
			contentReq.Header.Set("Range", rangeHeader)
		}
	}
	contentReq.Header.Set("Authorization", "Bearer "+token)
	account.ApplyHeaderOverrides(contentReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	contentResp, err := doAccountHTTPUpstream(s.httpUpstream, contentReq, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = contentResp.Body.Close() }()
	contentRequestID := firstNonEmpty(contentResp.Header.Get("x-request-id"), contentResp.Header.Get("xai-request-id"))
	if contentResp.StatusCode >= 300 && contentResp.StatusCode < 400 {
		location := strings.TrimSpace(contentResp.Header.Get("Location"))
		if location == "" {
			return nil, fmt.Errorf("multimodal media content redirect is missing Location")
		}
		// 重定向同样会把内容交付给客户端，必须在放行前完成结算，否则只取片
		// 不轮询的客户端可以绕过计费白拿视频。
		if err := s.settleMultimodalVideoContentBalance(ctx, c, account, requestID); err != nil {
			return nil, err
		}
		writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), contentResp.Header, s.responseHeaderFilter)
		c.Header("Location", location)
		c.Status(contentResp.StatusCode)
		MarkResponseCommitted(c)
		return &OpenAIForwardResult{
			RequestID:       contentRequestID,
			ResponseHeaders: contentResp.Header.Clone(),
			Duration:        time.Since(startTime),
		}, nil
	}
	if contentResp.StatusCode >= 400 && contentResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return s.handleMultimodalMediaErrorResponse(ctx, contentResp, c, account, contentRequestID, "")
	}
	if contentResp.StatusCode < 300 {
		// 内容真正交付前结算冻结；失败直接返回错误，不吐内容。
		if err := s.settleMultimodalVideoContentBalance(ctx, c, account, requestID); err != nil {
			return nil, err
		}
	}
	if err := writeMultimodalMediaContentResponse(c, contentResp); err != nil {
		return nil, err
	}
	return &OpenAIForwardResult{
		RequestID:       contentRequestID,
		ResponseHeaders: contentResp.Header.Clone(),
		Duration:        time.Since(startTime),
	}, nil
}

// normalizeMultimodalVideoReferenceBody 把视频编辑/续写端点常用的
// video / video_url / videos 字段统一改写为上游 /v1/videos/generations
// 认识的 referenceVideos，避免参考视频在直通链路上丢失。
func normalizeMultimodalVideoReferenceBody(endpoint MultimodalMediaEndpoint, body []byte) ([]byte, error) {
	if endpoint != MultimodalMediaEndpointVideosEdits && endpoint != MultimodalMediaEndpointVideosExtensions {
		return body, nil
	}
	if !gjson.ValidBytes(body) {
		return body, nil
	}
	references := make([]string, 0, 1)
	var appendValue func(value gjson.Result)
	appendValue = func(value gjson.Result) {
		if !value.Exists() {
			return
		}
		if value.IsArray() {
			for _, item := range value.Array() {
				appendValue(item)
			}
			return
		}
		if value.Type == gjson.String {
			if trimmed := strings.TrimSpace(value.String()); trimmed != "" {
				references = append(references, trimmed)
			}
			return
		}
		if trimmed := grokMediaJSONImageURL(value); trimmed != "" {
			references = append(references, trimmed)
		}
	}
	updated := body
	var err error
	if !gjson.GetBytes(body, "referenceVideos").Exists() {
		appendValue(gjson.GetBytes(body, "video_url"))
		appendValue(gjson.GetBytes(body, "video"))
		appendValue(gjson.GetBytes(body, "videos"))
		if len(references) == 0 {
			return body, nil
		}
		updated, err = sjson.SetBytes(body, "referenceVideos", references)
		if err != nil {
			return nil, fmt.Errorf("normalize multimodal video reference body: %w", err)
		}
	}
	for _, field := range []string{"video_url", "video", "videos"} {
		if !gjson.GetBytes(updated, field).Exists() {
			continue
		}
		if updated, err = sjson.DeleteBytes(updated, field); err != nil {
			return nil, fmt.Errorf("normalize multimodal video reference body: %w", err)
		}
	}
	return updated, nil
}

func multimodalMediaClientWantsStream(body []byte, contentType string) bool {
	if gjson.ValidBytes(body) {
		return gjson.GetBytes(body, "stream").Bool()
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return false
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err != nil {
			return false
		}
		if part.FormName() == "stream" && part.FileName() == "" {
			value, err := io.ReadAll(io.LimitReader(part, 16))
			return err == nil && strings.EqualFold(strings.TrimSpace(string(value)), "true")
		}
		_ = part.Close()
	}
}

// forceMultimodalImageUpstreamStream 为生图请求强制 stream=true。
// 上游公网域名经 Cloudflare，单请求 100 秒硬超时；同步生图（seedream 90~130s）
// 必须走 SSE 才不会中途被掐断。客户端是否要流式由后续响应阶段决定。
func forceMultimodalImageUpstreamStream(
	endpoint MultimodalMediaEndpoint,
	body []byte,
	contentType string,
) ([]byte, string, error) {
	if !endpoint.IsImageRequest() {
		return body, contentType, nil
	}
	if gjson.ValidBytes(body) {
		if gjson.GetBytes(body, "stream").Bool() {
			return body, contentType, nil
		}
		updated, err := sjson.SetBytes(body, "stream", true)
		if err != nil {
			return body, contentType, err
		}
		return updated, contentType, nil
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, nil
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return body, contentType, nil
	}
	rewritten, rewrittenType, err := appendMultipartFormField(body, boundary, "stream", "true")
	if err != nil {
		return body, contentType, nil
	}
	return rewritten, rewrittenType, nil
}

func appendMultipartFormField(body []byte, boundary, field, value string) ([]byte, string, error) {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
		if strings.EqualFold(part.FormName(), field) {
			continue
		}
		target, err := writer.CreatePart(part.Header)
		if err != nil {
			return nil, "", err
		}
		if _, err := io.Copy(target, part); err != nil {
			return nil, "", err
		}
	}
	if err := writer.WriteField(field, value); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func multimodalMediaIsEventStream(contentType string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(contentType)), "text/event-stream")
}

type multimodalMediaUsageMetadata struct {
	ResponseID       string
	Usage            OpenAIUsage
	ImageCount       int
	ImageSize        string
	ImageInputSize   string
	ImageOutputSizes []string
	// ImageBillingSizeLocked 表示图片计费档位锁定为请求档位，不允许落账时被上游尺寸覆盖。
	ImageBillingSizeLocked bool
	VideoCount             int
	VideoResolution        string
	VideoDurationSeconds   int
}

func multimodalMediaUsageFromResponse(
	endpoint MultimodalMediaEndpoint,
	requestInfo GrokMediaRequestInfo,
	requestBody []byte,
	responseBody []byte,
) multimodalMediaUsageMetadata {
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	meta := multimodalMediaUsageMetadata{Usage: usage}
	switch endpoint {
	case MultimodalMediaEndpointImagesGenerations, MultimodalMediaEndpointImagesEdits:
		meta.ImageCount = countOpenAIResponseImageOutputsFromJSONBytes(responseBody)
		// 提交上游前只按请求档位校验过价格：请求给出了可分类尺寸时锁定该档位，
		// 不允许落账时被上游输出尺寸改档；未指定尺寸时按上游实际输出计费。
		if tier, ok := ClassifyImageBillingTier(requestInfo.Size); ok {
			meta.ImageSize = tier
			meta.ImageBillingSizeLocked = true
		}
		meta.ImageInputSize = requestInfo.Size
		meta.ImageOutputSizes = collectOpenAIResponseImageOutputSizesFromJSONBytes(responseBody)
	case MultimodalMediaEndpointVideosGenerations, MultimodalMediaEndpointVideosEdits,
		MultimodalMediaEndpointVideosExtensions:
		meta.ResponseID = extractGrokMediaVideoRequestID(responseBody)
		meta.VideoCount = 1
		meta.VideoResolution = requestInfo.Resolution
		meta.VideoDurationSeconds = MultimodalVideoBillingDurationSeconds(requestInfo, requestBody)
		// Keep the legacy media-unit counter populated for existing usage displays.
		meta.ImageCount = 1
	}
	return meta
}

// MultimodalVideoBillingDurationSeconds 处理 duration=-1（仅 Seedance 可用）。
// 该值表示按模型最长时长生成，冻结时按 15 秒上限计算，完成后再按实际秒数结算。
func MultimodalVideoBillingDurationSeconds(requestInfo GrokMediaRequestInfo, requestBody []byte) int {
	if gjson.ValidBytes(requestBody) {
		if raw := gjson.GetBytes(requestBody, "duration"); raw.Exists() && raw.Int() < 0 {
			return VideoBillingMaxDurationSeconds
		}
	}
	return requestInfo.DurationSeconds
}

func (s *OpenAIGatewayService) handleMultimodalMediaErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	requestedModel string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, requestedModel)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("multimodal upstream returned status %d", resp.StatusCode)
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		MarkResponseCommitted(c)
		writeMultimodalMediaErrorResponse(c, status, errType, errMsg)
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeMultimodalMediaErrorResponse(c, http.StatusInternalServerError, "upstream_error", "Upstream gateway error")
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	kind := "http_error"
	if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, body) {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if kind == "failover" {
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			ResponseHeaders:        resp.Header.Clone(),
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	MarkResponseCommitted(c)
	writeMultimodalMediaErrorResponse(c, resp.StatusCode, multimodalMediaErrorType(resp.StatusCode), upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func multimodalMediaErrorType(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}

func writeMultimodalMediaErrorResponse(c *gin.Context, statusCode int, errType, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
		},
	})
}

func writeMultimodalMediaResponse(
	c *gin.Context,
	statusCode int,
	contentType string,
	body []byte,
	upstreamHeader http.Header,
	filter *responseheaders.CompiledHeaderFilter,
) {
	if c == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), upstreamHeader, filter)
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "application/json"
	}
	// Gin's Data renderer preserves an existing Content-Type, including the
	// upstream SSE value copied above. Explicitly replace it after aggregation.
	c.Header("Content-Type", contentType)
	// 聚合后长度已变化，上游 Content-Length 必须丢弃。
	c.Writer.Header().Del("Content-Length")
	c.Data(statusCode, contentType, body)
}

func writeMultimodalMediaContentResponse(c *gin.Context, resp *http.Response) error {
	if c == nil || resp == nil || resp.Body == nil {
		return fmt.Errorf("multimodal media content response is incomplete")
	}
	for _, name := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Content-Disposition",
	} {
		if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
			c.Header(name, value)
		}
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Length")) == "" && resp.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Type")) == "" {
		c.Header("Content-Type", "application/octet-stream")
	}
	c.Status(resp.StatusCode)
	MarkResponseCommitted(c)
	_, err := io.Copy(c.Writer, resp.Body)
	return err
}

// renderMultimodalVideoResponse 把上游视频任务响应整理成稳定结构，
// 避免把上游私有字段直接暴露给客户端：
//   - id / task_id 统一为同一个任务标识（上游可能只给 job_id）；
//   - 补 object=video、progress、created_at，创建阶段再补 model / seconds / size；
//   - 删除 quoted_price_usd（上游自己的报价，与本地计费无关）、重复的 job_id；
//   - 删除全零 usage（上游在任务创建阶段尚未产生真实用量）。
func renderMultimodalVideoResponse(
	endpoint MultimodalMediaEndpoint,
	requestInfo GrokMediaRequestInfo,
	requestBody []byte,
	responseBody []byte,
) []byte {
	if !gjson.ValidBytes(responseBody) {
		return responseBody
	}
	root := gjson.ParseBytes(responseBody)
	if !root.IsObject() {
		return responseBody
	}
	taskID := firstNonEmpty(
		strings.TrimSpace(root.Get("id").String()),
		strings.TrimSpace(root.Get("task_id").String()),
		strings.TrimSpace(root.Get("job_id").String()),
	)
	if taskID == "" {
		return responseBody
	}

	rendered := responseBody
	setField := func(path string, value any) {
		updated, err := sjson.SetBytes(rendered, path, value)
		if err == nil {
			rendered = updated
		}
	}
	deleteField := func(path string) {
		if !gjson.GetBytes(rendered, path).Exists() {
			return
		}
		updated, err := sjson.DeleteBytes(rendered, path)
		if err == nil {
			rendered = updated
		}
	}

	setField("id", taskID)
	setField("task_id", taskID)
	setField("object", "video")
	if !root.Get("created_at").Exists() {
		setField("created_at", time.Now().Unix())
	}
	if !root.Get("progress").Exists() {
		setField("progress", multimodalVideoProgressForStatus(root.Get("status").String()))
	}

	deleteField("quoted_price_usd")
	deleteField("job_id")
	if multimodalVideoUsageIsZero(root.Get("usage")) {
		deleteField("usage")
	}

	if endpoint.IsVideoGenerationRequest() {
		if !root.Get("model").Exists() && strings.TrimSpace(requestInfo.Model) != "" {
			setField("model", requestInfo.Model)
		}
		if !root.Get("seconds").Exists() {
			if seconds := multimodalVideoRequestedSeconds(requestBody); seconds > 0 {
				setField("seconds", seconds)
			}
		}
		if !root.Get("size").Exists() && strings.TrimSpace(requestInfo.Resolution) != "" {
			setField("size", requestInfo.Resolution)
		}
		return rendered
	}

	// 轮询阶段没有请求体，只能从上游响应里把等价字段改名到标准位置。
	if !root.Get("seconds").Exists() {
		if duration := root.Get("duration"); duration.Exists() && duration.Int() > 0 {
			setField("seconds", duration.Int())
		}
	}
	if !root.Get("size").Exists() {
		if size := strings.TrimSpace(root.Get("resolution").String()); size != "" {
			setField("size", size)
		}
	}
	return rendered
}

func multimodalVideoProgressForStatus(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "completed", "succeeded", "success":
		return 100
	default:
		return 0
	}
}

// multimodalVideoRequestedSeconds 只回显客户端显式请求的时长；
// duration=-1 表示按模型最长时长生成，实际值未知，因此不输出。
func multimodalVideoRequestedSeconds(requestBody []byte) int {
	if !gjson.ValidBytes(requestBody) {
		return 0
	}
	duration := gjson.GetBytes(requestBody, "duration")
	if !duration.Exists() || duration.Int() <= 0 {
		return 0
	}
	return int(duration.Int())
}

func multimodalVideoUsageIsZero(usage gjson.Result) bool {
	if !usage.Exists() || !usage.IsObject() {
		return false
	}
	zero := true
	usage.ForEach(func(_, item gjson.Result) bool {
		switch item.Type {
		case gjson.Number:
			if item.Float() != 0 {
				zero = false
				return false
			}
		case gjson.String:
			if strings.TrimSpace(item.String()) != "" {
				zero = false
				return false
			}
		case gjson.Null:
		default:
			// 嵌套结构或布尔值不参与零值判定，避免误删有效统计。
			zero = false
			return false
		}
		return true
	})
	return zero
}

// rewriteMultimodalVideoContentURLs 把上游自有的取片地址改写成本服务的代理地址，
// 避免把需要上游 API Key 的 URL 暴露给客户端。外部 CDN 地址保持原样。
func rewriteMultimodalVideoContentURLs(body []byte, requestID, proxyURL string) []byte {
	return rewriteGrokMediaVideoContentURLs(body, requestID, proxyURL)
}

func multimodalMediaContentProxyURL(c *gin.Context, requestID string) string {
	return grokMediaContentProxyURL(c, requestID)
}
