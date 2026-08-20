package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

type AgnesVideoEndpoint string

const (
	AgnesVideoEndpointGenerations AgnesVideoEndpoint = "generations"
	AgnesVideoEndpointStatus      AgnesVideoEndpoint = "status"
	AgnesVideoEndpointContent     AgnesVideoEndpoint = "content"
)

func (e AgnesVideoEndpoint) httpMethod() string {
	switch e {
	case AgnesVideoEndpointGenerations:
		return http.MethodPost
	case AgnesVideoEndpointStatus, AgnesVideoEndpointContent:
		return http.MethodGet
	default:
		return http.MethodPost
	}
}

func (e AgnesVideoEndpoint) RequiresRequestBody() bool {
	return e == AgnesVideoEndpointGenerations
}

func AgnesVideoTaskSessionHash(taskID string, userID, apiKeyID int64) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	ownerSeed := fmt.Sprintf("%d:%d:%s", userID, apiKeyID, taskID)
	return "agnes-video:" + DeriveSessionHashFromSeed(ownerSeed)
}

func (s *OpenAIGatewayService) BindAgnesVideoTaskAccount(
	ctx context.Context,
	groupID *int64,
	taskID string,
	userID, apiKeyID, accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("agnes video task binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(AgnesVideoTaskSessionHash(taskID, userID, apiKeyID))
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("agnes video task binding is invalid")
	}
	ttl := openaiStickySessionTTL
	if s.cfg != nil && s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds > 0 {
		ttl = time.Duration(s.cfg.Gateway.OpenAIWS.StickySessionTTLSeconds) * time.Second
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, accountID, ttl)
}

func (s *OpenAIGatewayService) ResolveAgnesVideoTaskAccount(
	ctx context.Context,
	groupID *int64,
	taskID string,
	userID, apiKeyID int64,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("agnes video task binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(AgnesVideoTaskSessionHash(taskID, userID, apiKeyID))
	if cacheKey == "" {
		return 0, fmt.Errorf("agnes video task binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

func buildAgnesVideoURL(account *Account, endpoint AgnesVideoEndpoint, taskID string) (string, error) {
	if account == nil {
		return "", fmt.Errorf("agnes account is required")
	}
	baseURL := account.GetBaseURL()
	if baseURL == "" {
		return "", fmt.Errorf("agnes account base_url is required")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	switch endpoint {
	case AgnesVideoEndpointGenerations:
		return baseURL + "/video/generations", nil
	case AgnesVideoEndpointStatus:
		if taskID == "" {
			return "", fmt.Errorf("task_id is required for status endpoint")
		}
		return fmt.Sprintf("%s/video/generations/%s", baseURL, url.PathEscape(taskID)), nil
	case AgnesVideoEndpointContent:
		if taskID == "" {
			return "", fmt.Errorf("task_id is required for content endpoint")
		}
		return fmt.Sprintf("%s/video/generations/%s/content", baseURL, url.PathEscape(taskID)), nil
	default:
		return "", fmt.Errorf("unsupported agnes video endpoint: %s", endpoint)
	}
}

func (s *OpenAIGatewayService) ForwardAgnesVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint AgnesVideoEndpoint,
	taskID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("agnes account is required")
	}
	if endpoint == AgnesVideoEndpointContent {
		token, _, err := s.getRequestCredential(ctx, c, account)
		if err != nil {
			return nil, err
		}
		return s.forwardAgnesVideoContent(ctx, c, account, token, taskID, startTime)
	}
	targetURL, err := buildAgnesVideoURL(account, endpoint, taskID)
	if err != nil {
		return nil, err
	}

	requestInfo := ParseAgnesVideoRequest(body)
	upstreamModel := requestInfo.Model
	if endpoint.RequiresRequestBody() && gjson.ValidBytes(body) {
		if mappedModel := strings.TrimSpace(account.GetMappedModel(upstreamModel)); mappedModel != "" {
			originalModel := upstreamModel
			upstreamModel = mappedModel
			body, err = sjson.SetBytes(body, "model", upstreamModel)
			if err != nil {
				return nil, fmt.Errorf("rewrite agnes video account mapped model: %w", err)
			}
			_ = originalModel
		}
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
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

	requestIDHeader := resp.Header.Get("x-request-id")
	requestModel := requestInfo.Model
	if resp.StatusCode >= 400 {
		return s.handleAgnesVideoErrorResponse(ctx, resp, c, account, requestIDHeader, upstreamModel)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}

	if endpoint == AgnesVideoEndpointStatus {
		respBody = rewriteAgnesVideoContentURLs(
			respBody,
			taskID,
			agnesVideoContentProxyURL(c, taskID),
		)
	}
	writeAgnesVideoResponse(c, resp, respBody, s.responseHeaderFilter)
	usage := agnesVideoUsageFromResponse(endpoint, requestInfo, respBody)
	return &OpenAIForwardResult{
		RequestID:            requestIDHeader,
		ResponseID:           usage.ResponseID,
		Usage:                usage.Usage,
		Model:                requestModel,
		BillingModel:         requestModel,
		UpstreamModel:        upstreamModel,
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		VideoCount:           usage.VideoCount,
		VideoResolution:      usage.VideoResolution,
		VideoDurationSeconds: usage.VideoDurationSeconds,
	}, nil
}

func (s *OpenAIGatewayService) handleAgnesVideoErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, requestID, upstreamModel string) (*OpenAIForwardResult, error) {
	respBody, _ := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	reqLog := logger.FromContext(ctx)
	reqLog.Error("agnes_video.upstream_error",
		zap.Int("status_code", resp.StatusCode),
		zap.String("response_body", string(respBody)),
		zap.String("request_id", requestID),
		zap.String("upstream_model", upstreamModel),
		zap.Int64("account_id", account.ID),
	)
	setOpsUpstreamError(c, resp.StatusCode, "Agnes video upstream error", truncateString(string(respBody), 512))
	failoverErr := &UpstreamFailoverError{
		StatusCode:               resp.StatusCode,
		ResponseBody:             respBody,
		ResponseHeaders:          resp.Header.Clone(),
		PreserveUpstreamResponse: true,
	}
	// Validation/model errors describe this request, not account health. Trying
	// the same payload on every account only turns the real upstream response
	// into a misleading "no available accounts" error.
	requestScopedModelError := strings.Contains(strings.ToLower(string(respBody)), "model is blocked") ||
		strings.Contains(strings.ToLower(string(respBody)), "model blocked")
	if requestScopedModelError || (resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError &&
		resp.StatusCode != http.StatusUnauthorized &&
		resp.StatusCode != http.StatusForbidden &&
		resp.StatusCode != http.StatusRequestTimeout &&
		resp.StatusCode != http.StatusTooManyRequests) {
		failoverErr.Scope = GatewayFailureScopeRequest
		failoverErr.NextAccountAction = NextAccountStop
	}
	return nil, failoverErr
}

func (s *OpenAIGatewayService) forwardAgnesVideoContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token, taskID string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	statusURL, err := buildAgnesVideoURL(account, AgnesVideoEndpointStatus, taskID)
	if err != nil {
		return nil, err
	}

	statusReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusReq.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(statusReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	statusResp, err := doAccountHTTPUpstream(s.httpUpstream, statusReq, proxyURL, account)
	if err != nil {
		return nil, fmt.Errorf("query agnes video status: %w", err)
	}
	defer func() { _ = statusResp.Body.Close() }()

	statusBody, err := ReadUpstreamResponseBody(statusResp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if statusResp.StatusCode != http.StatusOK {
		setOpsUpstreamError(c, statusResp.StatusCode, "Agnes video status query failed", truncateString(string(statusBody), 512))
		return nil, &UpstreamFailoverError{
			StatusCode:      statusResp.StatusCode,
			ResponseBody:    statusBody,
			ResponseHeaders: statusResp.Header.Clone(),
		}
	}

	videoURL := ""
	for _, path := range []string{"video.url", "video_url", "data.video_url", "url"} {
		if videoURL = strings.TrimSpace(gjson.GetBytes(statusBody, path).String()); videoURL != "" {
			break
		}
	}
	if videoURL == "" {
		return nil, fmt.Errorf("agnes video url not found in status response")
	}

	if !strings.HasPrefix(videoURL, "http://") && !strings.HasPrefix(videoURL, "https://") {
		return nil, fmt.Errorf("invalid agnes video url: %s", videoURL)
	}

	contentReq, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return nil, err
	}
	if rangeHeader := c.GetHeader("Range"); rangeHeader != "" {
		contentReq.Header.Set("Range", rangeHeader)
	}
	contentReq.Header.Set("Authorization", "Bearer "+token)
	account.ApplyHeaderOverrides(contentReq.Header)

	contentResp, err := doAccountHTTPUpstream(s.httpUpstream, contentReq, proxyURL, account)
	if err != nil {
		return nil, fmt.Errorf("download agnes video content: %w", err)
	}
	defer func() { _ = contentResp.Body.Close() }()

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition"} {
		if v := contentResp.Header.Get(h); v != "" {
			c.Header(h, v)
		}
	}
	c.Status(contentResp.StatusCode)
	_, _ = io.Copy(c.Writer, contentResp.Body)
	return &OpenAIForwardResult{
		ResponseHeaders: contentResp.Header.Clone(),
		Duration:        time.Since(startTime),
	}, nil
}

type AgnesVideoRequestInfo struct {
	Model      string
	Prompt     string
	Resolution string
	Duration   int
}

func ParseAgnesVideoRequest(body []byte) AgnesVideoRequestInfo {
	if !gjson.ValidBytes(body) {
		return AgnesVideoRequestInfo{}
	}
	parsed := gjson.ParseBytes(body)
	return AgnesVideoRequestInfo{
		Model:      parsed.Get("model").String(),
		Prompt:     parsed.Get("prompt").String(),
		Resolution: parsed.Get("resolution").String(),
		Duration:   int(parsed.Get("duration").Int()),
	}
}

type agnesVideoUsage struct {
	ResponseID           string
	Usage                OpenAIUsage
	VideoCount           int
	VideoResolution      string
	VideoDurationSeconds int
}

func agnesVideoUsageFromResponse(endpoint AgnesVideoEndpoint, requestInfo AgnesVideoRequestInfo, respBody []byte) agnesVideoUsage {
	var usage agnesVideoUsage
	if !gjson.ValidBytes(respBody) {
		return usage
	}
	parsed := gjson.ParseBytes(respBody)
	usage.ResponseID = extractAgnesVideoTaskID(parsed)
	if endpoint == AgnesVideoEndpointGenerations {
		usage.VideoCount = 1
		usage.VideoResolution = requestInfo.Resolution
		usage.VideoDurationSeconds = requestInfo.Duration
		if usage.VideoDurationSeconds <= 0 {
			usage.VideoDurationSeconds = 8
		}
	}
	return usage
}

func extractAgnesVideoTaskID(parsed gjson.Result) string {
	for _, key := range []string{"request_id", "id", "data.request_id", "data.id", "video.request_id", "video.id", "task_id", "data.task_id", "video.task_id"} {
		if id := parsed.Get(key).String(); id != "" {
			return id
		}
	}
	return ""
}

func rewriteAgnesVideoContentURLs(respBody []byte, taskID, proxyURL string) []byte {
	if !gjson.ValidBytes(respBody) {
		return respBody
	}
	result := respBody
	for _, path := range []string{"video.url", "video_url", "data.video_url", "url"} {
		if gjson.GetBytes(result, path).Exists() {
			var err error
			result, err = sjson.SetBytes(result, path, proxyURL)
			if err != nil {
				return respBody
			}
		}
	}
	return result
}

func agnesVideoContentProxyURL(c *gin.Context, taskID string) string {
	return fmt.Sprintf("/v1/videos/%s/content", taskID)
}

func writeAgnesVideoResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}
